-- 回滚服务版本登记与后端 OCI 部署目标 schema。
-- SQLite 不支持 DROP COLUMN（旧版本），重建索引与表以回滚。

DROP INDEX IF EXISTS idx_service_version_events_version;
DROP TABLE IF EXISTS service_version_events;

DROP INDEX IF EXISTS idx_service_versions_idempotency;

-- SQLite 3.35+ 支持 DROP COLUMN；go-sqlite3 驱动内置版本满足。
ALTER TABLE service_versions DROP COLUMN registration_request_hash;
ALTER TABLE service_versions DROP COLUMN registration_idempotency_key;

ALTER TABLE deployment_targets DROP COLUMN artifact_type;
