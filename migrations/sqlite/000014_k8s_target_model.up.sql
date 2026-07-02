-- Kubernetes Deployment executor target model.
-- SQLite is used for Go tests and mirrors the runtime schema shape.

CREATE TABLE ssh_deployment_targets (
  deployment_target_id TEXT PRIMARY KEY,
  target_type TEXT NOT NULL CHECK (target_type IN ('server', 'server_group')),
  target_ref_id TEXT NOT NULL,
  script_path TEXT NOT NULL DEFAULT '',
  working_dir TEXT NOT NULL DEFAULT '',
  env_vars TEXT NOT NULL DEFAULT '{}',
  FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id)
);

INSERT INTO ssh_deployment_targets (
  deployment_target_id, target_type, target_ref_id, script_path, working_dir, env_vars
)
SELECT id, target_type, target_ref_id, script_path, working_dir, env_vars
FROM deployment_targets
WHERE executor_type = 'ssh';

CREATE TABLE k8s_clusters (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  credential_ref TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE k8s_deployment_targets (
  deployment_target_id TEXT PRIMARY KEY,
  cluster_id TEXT NOT NULL,
  namespace TEXT NOT NULL,
  deployment_name TEXT NOT NULL,
  container_name TEXT NOT NULL,
  FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id),
  FOREIGN KEY (cluster_id) REFERENCES k8s_clusters(id)
);

CREATE TABLE deploy_target_logs (
  id TEXT PRIMARY KEY,
  deploy_record_id TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('server', 'server_group_member', 'k8s_deployment', 'mock')),
  target_ref_id TEXT NOT NULL,
  target_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed', 'skipped')),
  exit_code INTEGER,
  started_at TEXT,
  finished_at TEXT,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  log_output TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id)
);

INSERT INTO deploy_target_logs (
  id, deploy_record_id, target_type, target_ref_id, target_name, status,
  exit_code, started_at, finished_at, duration_ms, log_output, error_code, error_message
)
SELECT
  l.id, l.deploy_record_id, 'server', l.server_id, COALESCE(s.name, l.server_id), l.status,
  l.exit_code, l.started_at, l.finished_at, l.duration_ms, l.log_output, l.error_code, l.error_message
FROM server_deploy_logs l
LEFT JOIN servers s ON s.id = l.server_id;

CREATE INDEX idx_dtl_deploy ON deploy_target_logs(deploy_record_id);
CREATE INDEX idx_dtl_target ON deploy_target_logs(target_type, target_ref_id);

CREATE TABLE deployment_states (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  deployment_target_id TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_ref_id TEXT NOT NULL,
  service_version_id TEXT NOT NULL,
  deploy_record_id TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id),
  FOREIGN KEY (environment_id) REFERENCES environments(id),
  FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id),
  FOREIGN KEY (service_version_id) REFERENCES service_versions(id),
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id),
  UNIQUE (service_id, environment_id, deployment_target_id, target_type, target_ref_id)
);

INSERT INTO deployment_states (
  id, service_id, environment_id, deployment_target_id, target_type, target_ref_id,
  service_version_id, deploy_record_id, updated_at
)
SELECT
  sds.id, sds.service_id, sds.environment_id, rr.deployment_target_id, 'server', sds.server_id,
  sds.service_version_id, sds.deploy_record_id, sds.updated_at
FROM server_deployment_states sds
JOIN deploy_records dr ON dr.id = sds.deploy_record_id
JOIN release_requests rr ON rr.id = dr.release_request_id;

ALTER TABLE deploy_records RENAME COLUMN total_servers TO total_targets;
ALTER TABLE deploy_records RENAME COLUMN success_servers TO success_targets;
ALTER TABLE deploy_records RENAME COLUMN failed_servers TO failed_targets;
ALTER TABLE deploy_records RENAME COLUMN skipped_servers TO skipped_targets;

ALTER TABLE deployment_targets RENAME TO deployment_targets_old;

CREATE TABLE deployment_targets (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  executor_type TEXT NOT NULL CHECK (executor_type IN ('mock', 'ssh', 'k8s')),
  artifact_type TEXT NOT NULL DEFAULT 'version_only' CHECK (artifact_type IN ('version_only', 'oci_image')),
  timeout_seconds INTEGER NOT NULL DEFAULT 300,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id),
  FOREIGN KEY (environment_id) REFERENCES environments(id)
);

INSERT INTO deployment_targets (
  id, service_id, environment_id, executor_type, artifact_type, timeout_seconds, enabled, created_at, updated_at
)
SELECT id, service_id, environment_id, executor_type, artifact_type, timeout_seconds, enabled, created_at, updated_at
FROM deployment_targets_old;

DROP TABLE deployment_targets_old;

CREATE INDEX idx_deployment_targets_service_env ON deployment_targets(service_id, environment_id);

DROP TABLE server_deployment_states;
DROP TABLE server_deploy_logs;
