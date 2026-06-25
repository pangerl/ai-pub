package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"

	"ai-pub/internal/domain"
)

// GetServiceByProjectSlugAndServiceSlug 按项目 slug 与服务 slug 定位既有服务。
// 外部 CI 登记版本时不传内部 service_id，改用此方法定位服务。
func (s Store) GetServiceByProjectSlugAndServiceSlug(ctx context.Context, projectSlug, serviceSlug string) (domain.Service, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT s.id, s.project_id, s.name, s.slug, s.description, s.enabled, s.created_at, s.updated_at
FROM services s
JOIN projects p ON p.id = s.project_id
WHERE p.slug = ? AND s.slug = ?`, projectSlug, serviceSlug)
	item, err := scanService(row)
	return item, normalizeNotFound(err)
}

// FindServiceVersionByIdempotency 按登记幂等键查找版本，用于外部 CI 重试去重。
func (s Store) FindServiceVersionByIdempotency(ctx context.Context, serviceID, idempotencyKey string) (domain.ServiceVersion, error) {
	if idempotencyKey == "" {
		return domain.ServiceVersion{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, registration_idempotency_key, registration_request_hash, created_at
FROM service_versions
WHERE service_id = ? AND registration_idempotency_key = ?`, serviceID, idempotencyKey)
	item, err := scanServiceVersion(row)
	return item, normalizeNotFound(err)
}

// FindServiceVersionByServiceAndVersion 按服务与版本号查找版本，用于不同幂等键下的版本冲突判断。
func (s Store) FindServiceVersionByServiceAndVersion(ctx context.Context, serviceID, version string) (domain.ServiceVersion, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, service_id, version, commit_sha, artifact_url, source, metadata, created_by_type, created_by_id, registration_idempotency_key, registration_request_hash, created_at
FROM service_versions
WHERE service_id = ? AND version = ?`, serviceID, version)
	item, err := scanServiceVersion(row)
	return item, normalizeNotFound(err)
}

// RegistrationRequestHash 计算登记请求指纹。
// 仅由定位服务后的稳定登记内容（version、commit_sha、artifact_url 的规范化值）计算，
// metadata、built_at、运行链接等可变追溯信息不参与，确保 CI 重试不改变指纹。
func RegistrationRequestHash(version, commitSHA, artifactURL string) string {
	normalize := func(v string) string {
		return strings.TrimSpace(v)
	}
	h := sha256.Sum256([]byte(normalize(version) + "\x1f" + normalize(commitSHA) + "\x1f" + normalize(artifactURL)))
	return hex.EncodeToString(h[:])
}

// CreateServiceVersionEvent 写入服务版本登记审计事件。
func (s Store) CreateServiceVersionEvent(ctx context.Context, item domain.ServiceVersionEvent) (domain.ServiceVersionEvent, error) {
	if item.ID == "" {
		item.ID = domain.NewID("vevt")
	}
	if item.Metadata == "" {
		item.Metadata = "{}"
	}
	item.CreatedAt = nowUTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO service_version_events (
id, service_version_id, event_type, actor_type, actor_id, api_key_id, registration_idempotency_key, message, metadata, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceVersionID, item.EventType, item.ActorType, item.ActorID, nullString(item.APIKeyID), nullString(item.RegistrationIdempotencyKey), item.Message, item.Metadata, formatTime(item.CreatedAt))
	return item, err
}

// ListServiceVersionEvents 按版本列出登记审计事件，按时间正序。
func (s Store) ListServiceVersionEvents(ctx context.Context, serviceVersionID string) ([]domain.ServiceVersionEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, service_version_id, event_type, actor_type, actor_id, api_key_id, registration_idempotency_key, message, metadata, created_at
FROM service_version_events
WHERE service_version_id = ?
ORDER BY created_at ASC, id ASC`, serviceVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ServiceVersionEvent{}
	for rows.Next() {
		item, err := scanServiceVersionEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanServiceVersionEvent(row rowScanner) (domain.ServiceVersionEvent, error) {
	var item domain.ServiceVersionEvent
	var createdAt string
	var apiKeyID, idempotencyKey sql.NullString
	err := row.Scan(&item.ID, &item.ServiceVersionID, &item.EventType, &item.ActorType, &item.ActorID, &apiKeyID, &idempotencyKey, &item.Message, &item.Metadata, &createdAt)
	item.APIKeyID = nullStringValue(apiKeyID)
	item.RegistrationIdempotencyKey = nullStringValue(idempotencyKey)
	item.CreatedAt = parseTime(createdAt)
	return item, err
}
