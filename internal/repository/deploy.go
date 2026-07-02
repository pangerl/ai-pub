package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-pub/internal/domain"
)

// DeployListFilter 描述发布记录列表的服务端筛选与分页条件。空字段表示不限制。
// service_id/environment_id 需通过 JOIN release_requests 过滤（deploy_records 无此列）。
type DeployListFilter struct {
	ReleaseRequestID string
	ServiceID        string
	EnvironmentID    string
	Status           string // 逗号分隔多值
	Page             int
	PageSize         int
}

// DeployRecordListItem 在 DeployRecord 基础上附带父发布单的关键字段，
// 使前端无需再维护 releaseByID Map 即可展示执行上下文。
type DeployRecordListItem struct {
	domain.DeployRecord
	ReleaseServiceID        string `json:"release_service_id"`
	ReleaseEnvironmentID    string `json:"release_environment_id"`
	ReleaseServiceVersionID string `json:"release_service_version_id"`
}

// PagedDeploys 是分页发布记录列表的响应结构。
type PagedDeploys struct {
	Items    []DeployRecordListItem `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type ClaimedDeploy struct {
	Release domain.ReleaseRequest
	Record  domain.DeployRecord
	Target  domain.DeploymentTarget
	Version domain.ServiceVersion
	Servers []domain.Server
}

type ServerResult struct {
	Status       string `json:"status"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	DurationMS   int    `json:"duration_ms"`
	LogOutput    string `json:"log_output,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type RecoveredDeploy struct {
	RecordID  string
	ReleaseID string
}

const DeployLeaseDuration = 10 * time.Minute

var ErrLeaseLost = errors.New("deploy lease lost")

func (s Store) ClaimNextDeploy(ctx context.Context, workerID string) (ClaimedDeploy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
SELECT dr.id, dr.release_request_id, dr.status, dr.executor_type, dr.target_snapshot, dr.total_targets, dr.success_targets,
dr.failed_targets, dr.skipped_targets, dr.worker_id, dr.created_at, dr.updated_at
FROM deploy_records dr
JOIN release_requests rr ON rr.id = dr.release_request_id
JOIN environments env ON env.id = rr.environment_id
WHERE dr.status = 'queued' AND rr.status = 'queued'
AND NOT EXISTS (
  SELECT 1
  FROM deploy_target_logs candidate
  JOIN deploy_target_logs running ON running.target_type = candidate.target_type AND running.target_ref_id = candidate.target_ref_id
  JOIN deploy_records running_record ON running_record.id = running.deploy_record_id
  WHERE candidate.deploy_record_id = dr.id
    AND running_record.status = 'running'
    AND running.status IN ('queued', 'running')
)
AND env.release_frozen = 0
ORDER BY dr.created_at ASC, dr.id ASC
LIMIT 1`)
	record, err := scanDeployRecord(row)
	if err != nil {
		return ClaimedDeploy{}, normalizeNotFound(err)
	}
	now := nowUTC()
	leaseExpiresAt := now.Add(DeployLeaseDuration)
	res, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET status = 'running', worker_id = ?, lease_expires_at = ?, started_at = ?, heartbeat_at = ?, updated_at = ?
WHERE id = ? AND status = 'queued'`,
		workerID, formatTime(leaseExpiresAt), formatTime(now), formatTime(now), formatTime(now), record.ID)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ClaimedDeploy{}, err
	}
	if affected != 1 {
		return ClaimedDeploy{}, ErrNotFound
	}
	res, err = tx.ExecContext(ctx, `
UPDATE release_requests SET status = 'running', updated_at = ? WHERE id = ? AND status = 'queued'`,
		formatTime(now), record.ReleaseRequestID)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	affected, err = res.RowsAffected()
	if err != nil {
		return ClaimedDeploy{}, err
	}
	if affected != 1 {
		return ClaimedDeploy{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return ClaimedDeploy{}, err
	}

	release, err := s.GetReleaseRequest(ctx, record.ReleaseRequestID)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	version, err := s.GetServiceVersion(ctx, release.ServiceVersionID)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	target, servers, err := s.resolveDeploySnapshot(ctx, record, release.DeploymentTargetID)
	if err != nil {
		return ClaimedDeploy{}, err
	}
	record.Status = "running"
	record.WorkerID = workerID
	return ClaimedDeploy{Release: release, Record: record, Target: target, Version: version, Servers: servers}, nil
}

func (s Store) HeartbeatDeploy(ctx context.Context, deployRecordID, workerID string) error {
	now := nowUTC()
	res, err := s.db.ExecContext(ctx, `
UPDATE deploy_records
SET heartbeat_at = ?, lease_expires_at = ?, updated_at = ?
WHERE id = ? AND status = 'running' AND worker_id = ?`,
		formatTime(now), formatTime(now.Add(DeployLeaseDuration)), formatTime(now), deployRecordID, workerID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s Store) RecoverExpiredDeploys(ctx context.Context) ([]RecoveredDeploy, error) {
	now := nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
SELECT id, release_request_id
FROM deploy_records
WHERE status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < ?`, formatTime(now))
	if err != nil {
		return nil, err
	}
	candidates := []RecoveredDeploy{}
	for rows.Next() {
		var item RecoveredDeploy
		if err := rows.Scan(&item.RecordID, &item.ReleaseID); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	recovered := []RecoveredDeploy{}
	for _, item := range candidates {
		res, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET status = 'failed', finished_at = ?, error_summary = ?, updated_at = ?
WHERE id = ? AND status = 'running' AND lease_expires_at < ?`,
			formatTime(now), "worker lease expired", formatTime(now), item.RecordID, formatTime(now))
		if err != nil {
			return nil, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected != 1 {
			continue
		}
		recovered = append(recovered, item)
		if _, err := tx.ExecContext(ctx, `
UPDATE deploy_target_logs
SET status = 'failed', finished_at = ?, error_code = 'worker_lease_expired', error_message = ?
WHERE deploy_record_id = ? AND status IN ('queued', 'running')`,
			formatTime(now), "worker lease expired", item.RecordID); err != nil {
			return nil, err
		}
		var success, failed, skipped int
		if err := tx.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END), 0)
FROM deploy_target_logs WHERE deploy_record_id = ?`, item.RecordID).Scan(&success, &failed, &skipped); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET success_targets = ?, failed_targets = ?, skipped_targets = ?
WHERE id = ?`, success, failed, skipped, item.RecordID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE release_requests
SET status = 'failed', summary_status = 'failed', summary_message = ?, updated_at = ?
WHERE id = ? AND status = 'running'`,
			"worker lease expired", formatTime(now), item.ReleaseID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return recovered, nil
}

func (s Store) resolveDeploySnapshot(ctx context.Context, record domain.DeployRecord, targetID string) (domain.DeploymentTarget, []domain.Server, error) {
	var snapshot deployTargetSnapshot
	if err := json.Unmarshal([]byte(record.TargetSnapshot), &snapshot); err == nil && snapshot.DeploymentTarget.ID != "" && len(snapshot.Servers) > 0 {
		return snapshot.DeploymentTarget, snapshot.Servers, nil
	}
	var legacyTarget domain.DeploymentTarget
	if err := json.Unmarshal([]byte(record.TargetSnapshot), &legacyTarget); err == nil && legacyTarget.ID != "" {
		servers, err := s.ListDeployServers(ctx, record.ID)
		return legacyTarget, servers, err
	}
	target, err := s.GetDeploymentTarget(ctx, targetID)
	if err != nil {
		return domain.DeploymentTarget{}, nil, err
	}
	servers, err := s.ListDeployServers(ctx, record.ID)
	return target, servers, err
}

func (s Store) ListDeployServers(ctx context.Context, deployRecordID string) ([]domain.Server, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.name, s.host, s.port, s.username, s.auth_type, s.credential_ref, s.role, s.gateway_id, s.enabled, s.last_check_status, s.last_check_at, s.created_at, s.updated_at
FROM deploy_target_logs l
JOIN servers s ON s.id = l.target_ref_id
WHERE l.deploy_record_id = ?
ORDER BY l.id ASC`, deployRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Server{}
	for rows.Next() {
		item, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) MarkTargetFinished(ctx context.Context, deployRecordID string, targetRefID string, result ServerResult) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE deploy_target_logs
SET status = ?, exit_code = ?, finished_at = ?, duration_ms = ?, log_output = ?, error_code = ?, error_message = ?
WHERE deploy_record_id = ? AND target_ref_id = ?`,
		result.Status, nullableInt(result.ExitCode), formatTime(now), result.DurationMS, result.LogOutput, result.ErrorCode, result.ErrorMessage, deployRecordID, targetRefID)
	return err
}

func (s Store) MarkTargetRunning(ctx context.Context, deployRecordID string, targetRefID string) error {
	now := nowUTC()
	res, err := s.db.ExecContext(ctx, `
UPDATE deploy_target_logs
SET status = 'running', started_at = ?
WHERE deploy_record_id = ? AND target_ref_id = ? AND status = 'queued'`,
		formatTime(now), deployRecordID, targetRefID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s Store) MarkQueuedTargetsSkipped(ctx context.Context, deployRecordID string, message string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE deploy_target_logs
SET status = 'skipped', finished_at = ?, error_code = 'skipped_after_failure', error_message = ?
WHERE deploy_record_id = ? AND status IN ('queued', 'running')`,
		formatTime(now), message, deployRecordID)
	return err
}

func (s Store) FinishDeploy(ctx context.Context, deployRecordID string) (domain.DeployRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeployRecord{}, err
	}
	defer tx.Rollback()

	var releaseID string
	if err := tx.QueryRowContext(ctx, `SELECT release_request_id FROM deploy_records WHERE id = ?`, deployRecordID).Scan(&releaseID); err != nil {
		return domain.DeployRecord{}, err
	}
	var serviceID, environmentID, versionID, deploymentTargetID string
	if err := tx.QueryRowContext(ctx, `
SELECT service_id, environment_id, service_version_id, deployment_target_id FROM release_requests WHERE id = ?`, releaseID).Scan(&serviceID, &environmentID, &versionID, &deploymentTargetID); err != nil {
		return domain.DeployRecord{}, err
	}
	counts := map[string]int{"success": 0, "failed": 0, "skipped": 0}
	rows, err := tx.QueryContext(ctx, `SELECT status, COUNT(*) FROM deploy_target_logs WHERE deploy_record_id = ? GROUP BY status`, deployRecordID)
	if err != nil {
		return domain.DeployRecord{}, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return domain.DeployRecord{}, err
		}
		counts[status] = count
	}
	if err := rows.Close(); err != nil {
		return domain.DeployRecord{}, err
	}
	status := aggregateDeployStatus(counts["success"], counts["failed"], counts["skipped"])
	releaseStatus := status
	if status == "partial" {
		releaseStatus = "failed"
	}
	now := nowUTC()
	var errorSummary string
	if status != "success" {
		errorSummary = fmt.Sprintf("success=%d failed=%d skipped=%d", counts["success"], counts["failed"], counts["skipped"])
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET status = ?, success_targets = ?, failed_targets = ?, skipped_targets = ?, lease_expires_at = NULL, finished_at = ?, error_summary = ?, updated_at = ?
WHERE id = ?`,
		status, counts["success"], counts["failed"], counts["skipped"], formatTime(now), errorSummary, formatTime(now), deployRecordID); err != nil {
		return domain.DeployRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE release_requests
SET status = ?, summary_status = ?, summary_message = ?, updated_at = ?
WHERE id = ?`,
		releaseStatus, status, errorSummary, formatTime(now), releaseID); err != nil {
		return domain.DeployRecord{}, err
	}
	if counts["success"] > 0 {
		successRows, err := tx.QueryContext(ctx, `
SELECT target_type, target_ref_id FROM deploy_target_logs WHERE deploy_record_id = ? AND status = 'success'`, deployRecordID)
		if err != nil {
			return domain.DeployRecord{}, err
		}
		targets := make([]struct {
			targetType  string
			targetRefID string
		}, 0, counts["success"])
		for successRows.Next() {
			var target struct {
				targetType  string
				targetRefID string
			}
			if err := successRows.Scan(&target.targetType, &target.targetRefID); err != nil {
				successRows.Close()
				return domain.DeployRecord{}, err
			}
			targets = append(targets, target)
		}
		if err := successRows.Close(); err != nil {
			return domain.DeployRecord{}, err
		}
		for _, target := range targets {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM deployment_states
WHERE service_id = ? AND environment_id = ? AND deployment_target_id = ? AND target_type = ? AND target_ref_id = ?`,
				serviceID, environmentID, deploymentTargetID, target.targetType, target.targetRefID); err != nil {
				return domain.DeployRecord{}, err
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO deployment_states (
id, service_id, environment_id, deployment_target_id, target_type, target_ref_id, service_version_id, deploy_record_id, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				domain.NewID("state"), serviceID, environmentID, deploymentTargetID, target.targetType, target.targetRefID, versionID, deployRecordID, formatTime(now)); err != nil {
				return domain.DeployRecord{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.DeployRecord{}, err
	}
	return s.GetDeployRecord(ctx, deployRecordID)
}

func (s Store) GetDeployRecord(ctx context.Context, id string) (domain.DeployRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, release_request_id, status, executor_type, target_snapshot, total_targets, success_targets, failed_targets,
skipped_targets, worker_id, created_at, updated_at
FROM deploy_records WHERE id = ?`, id)
	item, err := scanDeployRecord(row)
	return item, normalizeNotFound(err)
}

func (s Store) ListDeployRecords(ctx context.Context, filter DeployListFilter) (PagedDeploys, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 50
	}

	// 始终 LEFT JOIN release_requests：既用于 service/env 筛选，也用于响应附带父发布单上下文。
	// JOIN 模式参照 ClaimNextDeploy。
	var clauses []string
	var args []any
	if filter.ReleaseRequestID != "" {
		clauses = append(clauses, "dr.release_request_id = ?")
		args = append(args, filter.ReleaseRequestID)
	}
	if filter.ServiceID != "" {
		clauses = append(clauses, "rr.service_id = ?")
		args = append(args, filter.ServiceID)
	}
	if filter.EnvironmentID != "" {
		clauses = append(clauses, "rr.environment_id = ?")
		args = append(args, filter.EnvironmentID)
	}
	if filter.Status != "" {
		statuses := strings.Split(filter.Status, ",")
		placeholders := make([]string, len(statuses))
		for i, st := range statuses {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(st))
		}
		clauses = append(clauses, "dr.status IN ("+strings.Join(placeholders, ", ")+")")
	}

	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT dr.id, dr.release_request_id, dr.status, dr.executor_type, dr.target_snapshot, dr.total_targets, dr.success_targets,
dr.failed_targets, dr.skipped_targets, dr.worker_id, dr.created_at, dr.updated_at,
rr.service_id AS release_service_id, rr.environment_id AS release_environment_id, rr.service_version_id AS release_service_version_id
FROM deploy_records dr LEFT JOIN release_requests rr ON rr.id = dr.release_request_id`+where+`
ORDER BY dr.created_at DESC, dr.id DESC
LIMIT ? OFFSET ?`,
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return PagedDeploys{}, err
	}
	defer rows.Close()
	items := []DeployRecordListItem{}
	for rows.Next() {
		item, err := scanDeployRecordListItem(rows)
		if err != nil {
			return PagedDeploys{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return PagedDeploys{}, err
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deploy_records dr LEFT JOIN release_requests rr ON rr.id = dr.release_request_id`+where, args...).Scan(&total); err != nil {
		return PagedDeploys{}, err
	}

	return PagedDeploys{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s Store) ListDeployTargetLogs(ctx context.Context, deployRecordID string) ([]domain.DeployTargetLog, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, deploy_record_id, target_type, target_ref_id, target_name, status, exit_code, started_at, finished_at, duration_ms, log_output, error_code, error_message
FROM deploy_target_logs WHERE deploy_record_id = ? ORDER BY id ASC`, deployRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DeployTargetLog{}
	for rows.Next() {
		item, err := scanDeployTargetLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) ListDeploymentStates(ctx context.Context) ([]domain.DeploymentState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, service_id, environment_id, deployment_target_id, target_type, target_ref_id, service_version_id, deploy_record_id, updated_at
FROM deployment_states ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DeploymentState{}
	for rows.Next() {
		var item domain.DeploymentState
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.ServiceID, &item.EnvironmentID, &item.DeploymentTargetID, &item.TargetType, &item.TargetRefID, &item.ServiceVersionID, &item.DeployRecordID, &updatedAt); err != nil {
			return nil, err
		}
		if item.TargetType == "server" || item.TargetType == "server_group_member" {
			item.ServerID = item.TargetRefID
		}
		item.UpdatedAt = parseTime(updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) RollbackCandidates(ctx context.Context, releaseID string) ([]domain.ServiceVersion, error) {
	release, err := s.GetReleaseRequest(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT sv.id, sv.service_id, sv.version, sv.commit_sha, sv.artifact_url, sv.source, sv.metadata, sv.created_by_type, sv.created_by_id, sv.registration_idempotency_key, sv.registration_request_hash, sv.created_at
FROM release_requests rr
JOIN deploy_records dr ON dr.release_request_id = rr.id
JOIN service_versions sv ON sv.id = rr.service_version_id
WHERE rr.service_id = ? AND rr.environment_id = ? AND dr.status = 'success' AND sv.id <> ?
ORDER BY rr.updated_at DESC, rr.id DESC
LIMIT 10`, release.ServiceID, release.EnvironmentID, release.ServiceVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ServiceVersion{}
	for rows.Next() {
		item, err := scanServiceVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type OpsSummary struct {
	QueuedDeploys         int `json:"queued_deploys"`
	RunningDeploys        int `json:"running_deploys"`
	FailedNotifications   int `json:"failed_notifications"`
	EnabledNotifications  int `json:"enabled_notifications"`
	TotalReleaseRequests  int `json:"total_release_requests"`
	TotalDeploymentStates int `json:"total_deployment_states"`
}

func (s Store) OpsSummary(ctx context.Context) (OpsSummary, error) {
	var out OpsSummary
	queries := []struct {
		target *int
		sql    string
	}{
		{&out.QueuedDeploys, `SELECT COUNT(*) FROM deploy_records WHERE status = 'queued'`},
		{&out.RunningDeploys, `SELECT COUNT(*) FROM deploy_records WHERE status = 'running'`},
		{&out.FailedNotifications, `SELECT COUNT(*) FROM notification_deliveries WHERE status = 'failed'`},
		{&out.EnabledNotifications, `SELECT COUNT(*) FROM notification_configs WHERE enabled = 1`},
		{&out.TotalReleaseRequests, `SELECT COUNT(*) FROM release_requests`},
		{&out.TotalDeploymentStates, `SELECT COUNT(*) FROM deployment_states`},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.sql).Scan(query.target); err != nil {
			return OpsSummary{}, err
		}
	}
	return out, nil
}

func scanDeployRecord(row rowScanner) (domain.DeployRecord, error) {
	var item domain.DeployRecord
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.ReleaseRequestID, &item.Status, &item.ExecutorType, &item.TargetSnapshot, &item.TotalTargets,
		&item.SuccessTargets, &item.FailedTargets, &item.SkippedTargets, &item.WorkerID, &createdAt, &updatedAt)
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

// scanDeployRecordListItem 扫描 12 列 deploy_record + 3 列父发布单上下文（LEFT JOIN 可能为 NULL）。
func scanDeployRecordListItem(row rowScanner) (DeployRecordListItem, error) {
	var item DeployRecordListItem
	var createdAt, updatedAt string
	var serviceID, environmentID, serviceVersionID sql.NullString
	err := row.Scan(&item.ID, &item.ReleaseRequestID, &item.Status, &item.ExecutorType, &item.TargetSnapshot, &item.TotalTargets,
		&item.SuccessTargets, &item.FailedTargets, &item.SkippedTargets, &item.WorkerID, &createdAt, &updatedAt,
		&serviceID, &environmentID, &serviceVersionID)
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	item.ReleaseServiceID = nullStringValue(serviceID)
	item.ReleaseEnvironmentID = nullStringValue(environmentID)
	item.ReleaseServiceVersionID = nullStringValue(serviceVersionID)
	return item, err
}

func scanDeployTargetLog(row rowScanner) (domain.DeployTargetLog, error) {
	var item domain.DeployTargetLog
	var exitCode sql.NullInt64
	var startedAt, finishedAt sql.NullString
	err := row.Scan(&item.ID, &item.DeployRecordID, &item.TargetType, &item.TargetRefID, &item.TargetName, &item.Status, &exitCode, &startedAt, &finishedAt,
		&item.DurationMS, &item.LogOutput, &item.ErrorCode, &item.ErrorMessage)
	if exitCode.Valid {
		value := int(exitCode.Int64)
		item.ExitCode = &value
	}
	if startedAt.Valid {
		item.StartedAt = parseTime(startedAt.String)
	}
	if finishedAt.Valid {
		item.FinishedAt = parseTime(finishedAt.String)
	}
	if item.TargetType == "server" || item.TargetType == "server_group_member" {
		item.ServerID = item.TargetRefID
	}
	return item, err
}

func aggregateDeployStatus(success int, failed int, skipped int) string {
	if success > 0 && (failed > 0 || skipped > 0) {
		return "partial"
	}
	if failed > 0 || skipped > 0 {
		return "failed"
	}
	return "success"
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
