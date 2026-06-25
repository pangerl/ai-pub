-- 服务版本登记与后端 OCI 部署目标：制品类型、登记幂等键与版本事件表。

-- 部署目标显式声明制品类型；存量部署目标默认 version_only，保持向后兼容。
ALTER TABLE deployment_targets
  ADD COLUMN artifact_type VARCHAR(32) NOT NULL DEFAULT 'version_only';

-- 限制本期支持的制品类型，避免脚本路径猜测。
ALTER TABLE deployment_targets
  ADD CONSTRAINT chk_deployment_targets_artifact_type
  CHECK (artifact_type IN ('version_only', 'oci_image'));

-- 服务版本登记幂等键与请求指纹，用于外部 CI 重试去重。
ALTER TABLE service_versions
  ADD COLUMN registration_idempotency_key VARCHAR(255) NULL,
  ADD COLUMN registration_request_hash VARCHAR(64) NULL;

-- (service_id, registration_idempotency_key) 唯一：手动登记幂等键为 NULL，多行空值不冲突。
CREATE UNIQUE INDEX idx_service_versions_idempotency
  ON service_versions (service_id, registration_idempotency_key);

-- 版本登记审计事件独立成表，不复用 release_events。
CREATE TABLE service_version_events (
  id VARCHAR(64) NOT NULL,
  service_version_id VARCHAR(64) NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  actor_type VARCHAR(32) NOT NULL,
  actor_id VARCHAR(64) NOT NULL DEFAULT '',
  api_key_id VARCHAR(64) NULL,
  registration_idempotency_key VARCHAR(255) NULL,
  message TEXT NOT NULL,
  metadata JSON NOT NULL,
  created_at DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_service_version_events_version (service_version_id, created_at),
  CONSTRAINT fk_service_version_events_version
    FOREIGN KEY (service_version_id) REFERENCES service_versions(id),
  CONSTRAINT chk_service_version_events_actor
    CHECK (actor_type IN ('user', 'api_key', 'system'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
