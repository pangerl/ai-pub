package repository

import (
	"context"
	"database/sql"
	"time"

	"ai-pub/internal/domain"
)

type NotificationSecret struct {
	Config     domain.NotificationConfig
	WebhookURL string
	Secret     string
}

type NotificationConfigPatch struct {
	Name       string
	WebhookURL string
	Secret     string
	Enabled    *bool
}

func (s Store) CreateNotificationConfig(ctx context.Context, item domain.NotificationConfig, webhookEnc string, secretEnc string) (domain.NotificationConfig, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("notif")
	}
	if item.Channel == "" {
		item.Channel = "wecom_robot"
	}
	item.Enabled = true
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO notification_configs (id, channel, name, webhook_url_enc, secret_enc, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Channel, item.Name, webhookEnc, secretEnc, boolInt(item.Enabled), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListNotificationConfigs(ctx context.Context) ([]domain.NotificationConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel, name, enabled, created_at, updated_at
FROM notification_configs ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.NotificationConfig{}
	for rows.Next() {
		item, err := scanNotificationConfig(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) ListEnabledNotificationSecrets(ctx context.Context) ([]NotificationSecret, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, channel, name, webhook_url_enc, secret_enc, enabled, created_at, updated_at
FROM notification_configs WHERE enabled = 1 ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []NotificationSecret{}
	for rows.Next() {
		item, err := scanNotificationSecret(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) GetNotificationSecret(ctx context.Context, id string) (NotificationSecret, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, channel, name, webhook_url_enc, secret_enc, enabled, created_at, updated_at
FROM notification_configs WHERE id = ? AND enabled = 1`, id)
	item, err := scanNotificationSecret(row)
	return item, normalizeNotFound(err)
}

func (s Store) UpdateNotificationConfig(ctx context.Context, id string, patch NotificationConfigPatch) (domain.NotificationConfig, error) {
	current, err := s.getNotificationSecretByID(ctx, id)
	if err != nil {
		return domain.NotificationConfig{}, err
	}
	if patch.Name != "" {
		current.Config.Name = patch.Name
	}
	if patch.WebhookURL != "" {
		current.WebhookURL = patch.WebhookURL
	}
	if patch.Secret != "" {
		current.Secret = patch.Secret
	}
	if patch.Enabled != nil {
		current.Config.Enabled = *patch.Enabled
	}
	current.Config.UpdatedAt = nowUTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE notification_configs
SET name = ?, webhook_url_enc = ?, secret_enc = ?, enabled = ?, updated_at = ?
WHERE id = ?`,
		current.Config.Name, current.WebhookURL, current.Secret, boolInt(current.Config.Enabled), formatTime(current.Config.UpdatedAt), id)
	if err != nil {
		return domain.NotificationConfig{}, err
	}
	return current.Config, nil
}

func (s Store) getNotificationSecretByID(ctx context.Context, id string) (NotificationSecret, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, channel, name, webhook_url_enc, secret_enc, enabled, created_at, updated_at
FROM notification_configs WHERE id = ?`, id)
	item, err := scanNotificationSecret(row)
	return item, normalizeNotFound(err)
}

func (s Store) CreateNotificationDelivery(ctx context.Context, item domain.NotificationDelivery) (domain.NotificationDelivery, error) {
	now := nowUTC()
	if item.ID == "" {
		item.ID = domain.NewID("ndel")
	}
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO notification_deliveries (
id, config_id, event_type, release_request_id, deploy_record_id, status, attempt_count, last_error, sent_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ConfigID, item.EventType, nullString(item.ReleaseRequestID), nullString(item.DeployRecordID), item.Status, item.AttemptCount, item.LastError, nullableTimeValue(item.SentAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	return item, err
}

func (s Store) ListNotificationDeliveries(ctx context.Context) ([]domain.NotificationDelivery, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, config_id, event_type, release_request_id, deploy_record_id, status, attempt_count, last_error, sent_at, created_at, updated_at
FROM notification_deliveries ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.NotificationDelivery{}
	for rows.Next() {
		item, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanNotificationConfig(row rowScanner) (domain.NotificationConfig, error) {
	var item domain.NotificationConfig
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Channel, &item.Name, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func scanNotificationSecret(row rowScanner) (NotificationSecret, error) {
	var item domain.NotificationConfig
	var webhookEnc, secretEnc string
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.Channel, &item.Name, &webhookEnc, &secretEnc, &enabled, &createdAt, &updatedAt)
	item.Enabled = enabled == 1
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return NotificationSecret{Config: item, WebhookURL: webhookEnc, Secret: secretEnc}, err
}

func scanNotificationDelivery(row rowScanner) (domain.NotificationDelivery, error) {
	var item domain.NotificationDelivery
	var releaseID, deployID, sentAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&item.ID, &item.ConfigID, &item.EventType, &releaseID, &deployID, &item.Status, &item.AttemptCount, &item.LastError, &sentAt, &createdAt, &updatedAt)
	item.ReleaseRequestID = nullStringValue(releaseID)
	item.DeployRecordID = nullStringValue(deployID)
	if sentAt.Valid {
		item.SentAt = parseTime(sentAt.String)
	}
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, err
}

func nullableTimeValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}
