package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"ai-pub/internal/domain"
)

func (s Store) GetUser(ctx context.Context, id string) (domain.User, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, username, display_name, role, enabled, created_at, updated_at FROM users WHERE id = ?`, id)
	item, err := scanUser(row)
	return item, normalizeNotFound(err)
}

func (s Store) GetServiceVersion(ctx context.Context, id string) (domain.ServiceVersion, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, created_at
FROM service_versions WHERE id = ?`, id)
	item, err := scanServiceVersion(row)
	return item, normalizeNotFound(err)
}

func (s Store) FindReleaseByIdempotency(ctx context.Context, key string) (domain.ReleaseRequest, error) {
	if key == "" {
		return domain.ReleaseRequest{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, project_id, service_id, environment_id, service_version_id, deployment_target_id, status, source, idempotency_key,
created_by_type, created_by_id, authorized_by_user_id, confirmed_by_user_id, confirmed_at, rejected_by_user_id, rejected_reason,
rollback_of_id, summary_status, summary_message, metadata, created_at, updated_at
FROM release_requests WHERE idempotency_key = ?`, key)
	item, err := scanReleaseRequest(row)
	return item, normalizeNotFound(err)
}

func (s Store) CreateReleaseRequest(ctx context.Context, item domain.ReleaseRequest) (domain.ReleaseRequest, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("rel")
	}
	if item.Source == "" {
		item.Source = "web"
	}
	if item.Status == "" {
		item.Status = "pending_confirm"
	}
	if item.Metadata == "" {
		item.Metadata = "{}"
	}
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO release_requests (
id, project_id, service_id, environment_id, service_version_id, deployment_target_id, status, source, idempotency_key,
created_by_type, created_by_id, authorized_by_user_id, rejected_reason, rollback_of_id, summary_status, summary_message, metadata, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ProjectID, item.ServiceID, item.EnvironmentID, item.ServiceVersionID, item.DeploymentTargetID, item.Status, item.Source, nullString(item.IdempotencyKey),
		item.CreatedByType, item.CreatedByID, nullString(item.AuthorizedByUserID), item.RejectedReason, nullString(item.RollbackOfID), item.SummaryStatus, item.SummaryMessage, item.Metadata, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListReleaseRequests(ctx context.Context) ([]domain.ReleaseRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, service_id, environment_id, service_version_id, deployment_target_id, status, source, idempotency_key,
created_by_type, created_by_id, authorized_by_user_id, confirmed_by_user_id, confirmed_at, rejected_by_user_id, rejected_reason,
rollback_of_id, summary_status, summary_message, metadata, created_at, updated_at
FROM release_requests ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ReleaseRequest{}
	for rows.Next() {
		item, err := scanReleaseRequest(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetReleaseRequest(ctx context.Context, id string) (domain.ReleaseRequest, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, project_id, service_id, environment_id, service_version_id, deployment_target_id, status, source, idempotency_key,
created_by_type, created_by_id, authorized_by_user_id, confirmed_by_user_id, confirmed_at, rejected_by_user_id, rejected_reason,
rollback_of_id, summary_status, summary_message, metadata, created_at, updated_at
FROM release_requests WHERE id = ?`, id)
	item, err := scanReleaseRequest(row)
	return item, normalizeNotFound(err)
}

func (s Store) SetReleaseRollbackOf(ctx context.Context, id string, rollbackOfID string) (domain.ReleaseRequest, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE release_requests SET rollback_of_id = ?, updated_at = ? WHERE id = ?`,
		rollbackOfID, formatTime(nowUTC()), id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	return s.GetReleaseRequest(ctx, id)
}

func (s Store) CountRunningReleases(ctx context.Context, serviceID string, environmentID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM release_requests
WHERE service_id = ? AND environment_id = ? AND status = 'running'`, serviceID, environmentID).Scan(&count)
	return count, err
}

func (s Store) RejectRelease(ctx context.Context, id string, userID string, reason string) (domain.ReleaseRequest, error) {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE release_requests
SET status = 'rejected', rejected_by_user_id = NULLIF(?, ''), rejected_reason = ?, updated_at = ?
WHERE id = ? AND status = 'pending_confirm'`, userID, reason, formatTime(now), id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	return s.GetReleaseRequest(ctx, id)
}

func (s Store) CancelRelease(ctx context.Context, id string, userID string) (domain.ReleaseRequest, error) {
	now := nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
UPDATE release_requests
SET status = 'cancelled', summary_message = ?, updated_at = ?
WHERE id = ? AND status IN ('pending_confirm', 'queued')`, "cancelled by "+userID, formatTime(now), id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if affected == 1 {
		if _, err := tx.ExecContext(ctx, `
UPDATE deploy_records
SET status = 'cancelled', finished_at = ?, error_summary = ?, updated_at = ?
WHERE release_request_id = ? AND status = 'queued'`,
			formatTime(now), "cancelled by "+userID, formatTime(now), id); err != nil {
			return domain.ReleaseRequest{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.ReleaseRequest{}, err
	}
	return s.GetReleaseRequest(ctx, id)
}

func (s Store) ConfirmAndQueueRelease(ctx context.Context, releaseID string, userID string, target domain.DeploymentTarget) (domain.ReleaseRequest, domain.DeployRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	defer tx.Rollback()

	now := nowUTC()
	serverIDs, err := s.expandTargetServersTx(ctx, tx, target)
	if err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	servers, err := s.getServersByIDsTx(ctx, tx, serverIDs)
	if err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	record := domain.DeployRecord{
		ID:               domain.NewID("deploy"),
		ReleaseRequestID: releaseID,
		Status:           "queued",
		ExecutorType:     target.ExecutorType,
		TargetSnapshot:   targetSnapshot(target, servers),
		TotalServers:     len(serverIDs),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	res, err := tx.ExecContext(ctx, `
UPDATE release_requests
SET status = 'queued', confirmed_by_user_id = NULLIF(?, ''), confirmed_at = ?, updated_at = ?
WHERE id = ? AND status = 'pending_confirm'`,
		userID, formatTime(now), formatTime(now), releaseID)
	if err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	if affected != 1 {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, fmt.Errorf("release is not pending_confirm")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO deploy_records (
id, release_request_id, status, executor_type, target_snapshot, total_servers, success_servers, failed_servers, skipped_servers,
worker_id, error_summary, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, '', '', ?, ?)`,
		record.ID, record.ReleaseRequestID, record.Status, record.ExecutorType, record.TargetSnapshot, record.TotalServers, formatTime(record.CreatedAt), formatTime(record.UpdatedAt)); err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	for _, serverID := range serverIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO server_deploy_logs (id, deploy_record_id, server_id, status, log_output, error_code, error_message)
VALUES (?, ?, ?, 'queued', '', '', '')`, domain.NewID("slog"), record.ID, serverID); err != nil {
			return domain.ReleaseRequest{}, domain.DeployRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.ReleaseRequest{}, domain.DeployRecord{}, err
	}
	release, err := s.GetReleaseRequest(ctx, releaseID)
	return release, record, err
}

func (s Store) CreateReleaseEvent(ctx context.Context, item domain.ReleaseEvent) (domain.ReleaseEvent, error) {
	if item.ID == "" {
		item.ID = domain.NewID("evt")
	}
	if item.Metadata == "" {
		item.Metadata = "{}"
	}
	item.CreatedAt = nowUTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO release_events (
id, release_request_id, deploy_record_id, event_type, actor_type, actor_id, authorized_user_id, api_key_id, source_ip, message, metadata, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, nullString(item.ReleaseRequestID), nullString(item.DeployRecordID), item.EventType, item.ActorType, item.ActorID, nullString(item.AuthorizedUserID), nullString(item.APIKeyID), item.SourceIP, item.Message, item.Metadata, formatTime(item.CreatedAt))
	return item, err
}

func (s Store) ListReleaseEvents(ctx context.Context, releaseID string) ([]domain.ReleaseEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, release_request_id, deploy_record_id, event_type, actor_type, actor_id, authorized_user_id, api_key_id, source_ip, message, metadata, created_at
FROM release_events WHERE release_request_id = ? ORDER BY event_seq ASC`, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ReleaseEvent{}
	for rows.Next() {
		item, err := scanReleaseEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) expandTargetServersTx(ctx context.Context, tx *sql.Tx, target domain.DeploymentTarget) ([]string, error) {
	if target.TargetType == "server" {
		return []string{target.TargetRefID}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT server_id FROM server_group_members WHERE server_group_id = ? ORDER BY server_id`, target.TargetRefID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("deployment target has no servers")
	}
	return ids, nil
}

func (s Store) getServersByIDsTx(ctx context.Context, tx *sql.Tx, ids []string) ([]domain.Server, error) {
	servers := make([]domain.Server, 0, len(ids))
	for _, id := range ids {
		row := tx.QueryRowContext(ctx, `
SELECT id, name, host, port, username, auth_type, credential_ref, gateway_id, enabled, last_check_status, last_check_at, created_at, updated_at
FROM servers WHERE id = ?`, id)
		server, err := scanServer(row)
		if err != nil {
			return nil, normalizeNotFound(err)
		}
		servers = append(servers, server)
	}
	return servers, nil
}

type deployTargetSnapshot struct {
	DeploymentTarget domain.DeploymentTarget `json:"deployment_target"`
	Servers          []domain.Server         `json:"servers"`
}

func targetSnapshot(target domain.DeploymentTarget, servers []domain.Server) string {
	body, err := json.Marshal(deployTargetSnapshot{DeploymentTarget: target, Servers: servers})
	if err != nil {
		return "{}"
	}
	return string(body)
}

func scanReleaseRequest(row rowScanner) (domain.ReleaseRequest, error) {
	var item domain.ReleaseRequest
	var idempotencyKey, authorizedBy, confirmedBy, confirmedAt, rejectedBy, rollbackOf sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.ProjectID, &item.ServiceID, &item.EnvironmentID, &item.ServiceVersionID, &item.DeploymentTargetID, &item.Status, &item.Source, &idempotencyKey,
		&item.CreatedByType, &item.CreatedByID, &authorizedBy, &confirmedBy, &confirmedAt, &rejectedBy, &item.RejectedReason,
		&rollbackOf, &item.SummaryStatus, &item.SummaryMessage, &item.Metadata, &createdAt, &updatedAt,
	)
	item.IdempotencyKey = nullStringValue(idempotencyKey)
	item.AuthorizedByUserID = nullStringValue(authorizedBy)
	item.ConfirmedByUserID = nullStringValue(confirmedBy)
	if confirmedAt.Valid {
		item.ConfirmedAt = parseTime(confirmedAt.String)
	}
	item.RejectedByUserID = nullStringValue(rejectedBy)
	item.RollbackOfID = nullStringValue(rollbackOf)
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanReleaseEvent(row rowScanner) (domain.ReleaseEvent, error) {
	var item domain.ReleaseEvent
	var releaseID, deployID, authorizedUserID, apiKeyID sql.NullString
	var createdAt string
	err := row.Scan(&item.ID, &releaseID, &deployID, &item.EventType, &item.ActorType, &item.ActorID, &authorizedUserID, &apiKeyID, &item.SourceIP, &item.Message, &item.Metadata, &createdAt)
	item.ReleaseRequestID = nullStringValue(releaseID)
	item.DeployRecordID = nullStringValue(deployID)
	item.AuthorizedUserID = nullStringValue(authorizedUserID)
	item.APIKeyID = nullStringValue(apiKeyID)
	item.CreatedAt = parseTime(createdAt)
	return item, err
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
