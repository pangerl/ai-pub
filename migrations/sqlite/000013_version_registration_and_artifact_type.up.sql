-- 服务版本登记与后端 OCI 部署目标：制品类型、登记幂等键与版本事件表。
-- SQLite 仅用于 Go 单测的内存数据库，schema 与 MySQL 运行时保持同构。

-- 部署目标显式声明制品类型；存量部署目标默认 version_only，保持向后兼容。
-- SQLite 的 ALTER TABLE 不支持直接加 CHECK 约束，改为在应用层校验取值。
ALTER TABLE deployment_targets
  ADD COLUMN artifact_type TEXT NOT NULL DEFAULT 'version_only';

-- 服务版本登记幂等键与请求指纹，用于外部 CI 重试去重。
ALTER TABLE service_versions
  ADD COLUMN registration_idempotency_key TEXT;
ALTER TABLE service_versions
  ADD COLUMN registration_request_hash TEXT;

-- (service_id, registration_idempotency_key) 唯一：手动登记幂等键为 NULL，SQLite 下多行 NULL 不冲突。
CREATE UNIQUE INDEX idx_service_versions_idempotency
  ON service_versions (service_id, registration_idempotency_key);

-- 版本登记审计事件独立成表，不复用 release_events。
CREATE TABLE service_version_events (
  id TEXT PRIMARY KEY,
  service_version_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'api_key', 'system')),
  actor_id TEXT NOT NULL DEFAULT '',
  api_key_id TEXT,
  registration_idempotency_key TEXT,
  message TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  FOREIGN KEY (service_version_id) REFERENCES service_versions(id)
);

CREATE INDEX idx_service_version_events_version ON service_version_events(service_version_id, created_at);
