CREATE TABLE server_deploy_logs (
  id TEXT PRIMARY KEY,
  deploy_record_id TEXT NOT NULL,
  server_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed', 'skipped')),
  exit_code INTEGER,
  started_at TEXT,
  finished_at TEXT,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  log_output TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

INSERT INTO server_deploy_logs (
  id, deploy_record_id, server_id, status, exit_code, started_at, finished_at,
  duration_ms, log_output, error_code, error_message
)
SELECT
  id, deploy_record_id, target_ref_id, status, exit_code, started_at, finished_at,
  duration_ms, log_output, error_code, error_message
FROM deploy_target_logs
WHERE target_type IN ('server', 'server_group_member');

CREATE TABLE server_deployment_states (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  server_id TEXT NOT NULL,
  service_version_id TEXT NOT NULL,
  deploy_record_id TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id),
  FOREIGN KEY (environment_id) REFERENCES environments(id),
  FOREIGN KEY (server_id) REFERENCES servers(id),
  FOREIGN KEY (service_version_id) REFERENCES service_versions(id),
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id),
  UNIQUE (service_id, environment_id, server_id)
);

INSERT INTO server_deployment_states (
  id, service_id, environment_id, server_id, service_version_id, deploy_record_id, updated_at
)
SELECT
  id, service_id, environment_id, target_ref_id, service_version_id, deploy_record_id, updated_at
FROM deployment_states
WHERE target_type IN ('server', 'server_group_member');

ALTER TABLE deploy_records RENAME COLUMN total_targets TO total_servers;
ALTER TABLE deploy_records RENAME COLUMN success_targets TO success_servers;
ALTER TABLE deploy_records RENAME COLUMN failed_targets TO failed_servers;
ALTER TABLE deploy_records RENAME COLUMN skipped_targets TO skipped_servers;

DROP TABLE deployment_states;
DROP TABLE deploy_target_logs;
DROP TABLE k8s_deployment_targets;
DROP TABLE k8s_clusters;
DROP TABLE ssh_deployment_targets;
