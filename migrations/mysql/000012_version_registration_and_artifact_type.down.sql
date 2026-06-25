-- 回滚服务版本登记与后端 OCI 部署目标 schema。

DROP TABLE IF EXISTS service_version_events;

DROP INDEX IF EXISTS idx_service_versions_idempotency ON service_versions;

ALTER TABLE service_versions
  DROP COLUMN registration_request_hash,
  DROP COLUMN registration_idempotency_key;

ALTER TABLE deployment_targets
  DROP CONSTRAINT chk_deployment_targets_artifact_type;

ALTER TABLE deployment_targets
  DROP COLUMN artifact_type;
