package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-pub/internal/domain"
)

type ClaimedDeploy struct {
	Release domain.ReleaseRequest
	Record  domain.DeployRecord
	Target  domain.DeploymentTarget
	Version domain.ServiceVersion
	Servers []domain.Server
}

type ServerResult struct {
	Status       string
	ExitCode     *int
	DurationMS   int
	LogOutput    string
	ErrorCode    string
	ErrorMessage string
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
SELECT dr.id, dr.release_request_id, dr.status, dr.executor_type, dr.target_snapshot, dr.total_servers, dr.success_servers,
dr.failed_servers, dr.skipped_servers, dr.worker_id, dr.created_at, dr.updated_at
FROM deploy_records dr
JOIN release_requests rr ON rr.id = dr.release_request_id
WHERE dr.status = 'queued' AND rr.status = 'queued'
AND NOT EXISTS (
  SELECT 1
  FROM server_deploy_logs candidate
  JOIN server_deploy_logs running ON running.server_id = candidate.server_id
  JOIN deploy_records running_record ON running_record.id = running.deploy_record_id
  WHERE candidate.deploy_record_id = dr.id
    AND running_record.status = 'running'
    AND running.status = 'running'
)
AND COALESCE(
  (SELECT manual_freeze_enabled FROM release_policies WHERE scope_type = 'service' AND scope_id = rr.service_id),
  (SELECT manual_freeze_enabled FROM release_policies WHERE scope_type = 'environment' AND scope_id = rr.environment_id),
  (SELECT manual_freeze_enabled FROM release_policies WHERE scope_type = 'system' AND scope_id = ''),
  0
) = 0
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
	if _, err := tx.ExecContext(ctx, `
UPDATE server_deploy_logs SET status = 'running', started_at = ? WHERE deploy_record_id = ? AND status = 'queued'`,
		formatTime(now), record.ID); err != nil {
		return ClaimedDeploy{}, err
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
UPDATE server_deploy_logs
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
FROM server_deploy_logs WHERE deploy_record_id = ?`, item.RecordID).Scan(&success, &failed, &skipped); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET success_servers = ?, failed_servers = ?, skipped_servers = ?
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
SELECT s.id, s.name, s.host, s.port, s.username, s.auth_type, s.credential_ref, s.gateway_id, s.enabled, s.last_check_status, s.last_check_at, s.created_at, s.updated_at
FROM server_deploy_logs l
JOIN servers s ON s.id = l.server_id
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

func (s Store) MarkServerFinished(ctx context.Context, deployRecordID string, serverID string, result ServerResult) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE server_deploy_logs
SET status = ?, exit_code = ?, finished_at = ?, duration_ms = ?, log_output = ?, error_code = ?, error_message = ?
WHERE deploy_record_id = ? AND server_id = ?`,
		result.Status, nullableInt(result.ExitCode), formatTime(now), result.DurationMS, result.LogOutput, result.ErrorCode, result.ErrorMessage, deployRecordID, serverID)
	return err
}

func (s Store) MarkQueuedServersSkipped(ctx context.Context, deployRecordID string, message string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE server_deploy_logs
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
	var serviceID, environmentID, versionID string
	if err := tx.QueryRowContext(ctx, `
SELECT service_id, environment_id, service_version_id FROM release_requests WHERE id = ?`, releaseID).Scan(&serviceID, &environmentID, &versionID); err != nil {
		return domain.DeployRecord{}, err
	}
	counts := map[string]int{"success": 0, "failed": 0, "skipped": 0}
	rows, err := tx.QueryContext(ctx, `SELECT status, COUNT(*) FROM server_deploy_logs WHERE deploy_record_id = ? GROUP BY status`, deployRecordID)
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
SET status = ?, success_servers = ?, failed_servers = ?, skipped_servers = ?, lease_expires_at = NULL, finished_at = ?, error_summary = ?, updated_at = ?
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
SELECT server_id FROM server_deploy_logs WHERE deploy_record_id = ? AND status = 'success'`, deployRecordID)
		if err != nil {
			return domain.DeployRecord{}, err
		}
		serverIDs := make([]string, 0, counts["success"])
		for successRows.Next() {
			var serverID string
			if err := successRows.Scan(&serverID); err != nil {
				successRows.Close()
				return domain.DeployRecord{}, err
			}
			serverIDs = append(serverIDs, serverID)
		}
		if err := successRows.Close(); err != nil {
			return domain.DeployRecord{}, err
		}
		for _, serverID := range serverIDs {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM server_deployment_states WHERE service_id = ? AND environment_id = ? AND server_id = ?`,
				serviceID, environmentID, serverID); err != nil {
				return domain.DeployRecord{}, err
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO server_deployment_states (id, service_id, environment_id, server_id, service_version_id, deploy_record_id, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
				domain.NewID("state"), serviceID, environmentID, serverID, versionID, deployRecordID, formatTime(now)); err != nil {
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
SELECT id, release_request_id, status, executor_type, target_snapshot, total_servers, success_servers, failed_servers,
skipped_servers, worker_id, created_at, updated_at
FROM deploy_records WHERE id = ?`, id)
	item, err := scanDeployRecord(row)
	return item, normalizeNotFound(err)
}

func (s Store) ListDeployRecords(ctx context.Context) ([]domain.DeployRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, release_request_id, status, executor_type, target_snapshot, total_servers, success_servers, failed_servers,
skipped_servers, worker_id, created_at, updated_at
FROM deploy_records ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DeployRecord{}
	for rows.Next() {
		item, err := scanDeployRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) ListServerDeployLogs(ctx context.Context, deployRecordID string) ([]domain.ServerDeployLog, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, deploy_record_id, server_id, status, exit_code, started_at, finished_at, duration_ms, log_output, error_code, error_message
FROM server_deploy_logs WHERE deploy_record_id = ? ORDER BY id ASC`, deployRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ServerDeployLog{}
	for rows.Next() {
		item, err := scanServerDeployLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) ListServerDeploymentStates(ctx context.Context) ([]domain.ServerDeploymentState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, service_id, environment_id, server_id, service_version_id, deploy_record_id, updated_at
FROM server_deployment_states ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ServerDeploymentState{}
	for rows.Next() {
		var item domain.ServerDeploymentState
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.ServiceID, &item.EnvironmentID, &item.ServerID, &item.ServiceVersionID, &item.DeployRecordID, &updatedAt); err != nil {
			return nil, err
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
SELECT sv.id, sv.service_id, sv.version, sv.commit_sha, sv.artifact_url, sv.source, sv.metadata, sv.created_by_type, sv.created_by_id, sv.created_at
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
		{&out.TotalDeploymentStates, `SELECT COUNT(*) FROM server_deployment_states`},
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
	err := row.Scan(&item.ID, &item.ReleaseRequestID, &item.Status, &item.ExecutorType, &item.TargetSnapshot, &item.TotalServers,
		&item.SuccessServers, &item.FailedServers, &item.SkippedServers, &item.WorkerID, &createdAt, &updatedAt)
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanServerDeployLog(row rowScanner) (domain.ServerDeployLog, error) {
	var item domain.ServerDeployLog
	var exitCode sql.NullInt64
	var startedAt, finishedAt sql.NullString
	err := row.Scan(&item.ID, &item.DeployRecordID, &item.ServerID, &item.Status, &exitCode, &startedAt, &finishedAt,
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
