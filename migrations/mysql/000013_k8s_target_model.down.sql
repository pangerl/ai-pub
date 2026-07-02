CREATE TABLE server_deploy_logs (
  id VARCHAR(64) PRIMARY KEY,
  deploy_record_id VARCHAR(64) NOT NULL,
  server_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  exit_code INT,
  started_at VARCHAR(64),
  finished_at VARCHAR(64),
  duration_ms INT NOT NULL DEFAULT 0,
  log_output MEDIUMTEXT NOT NULL,
  error_code VARCHAR(64) NOT NULL,
  error_message TEXT NOT NULL
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
  id VARCHAR(64) PRIMARY KEY,
  service_id VARCHAR(64) NOT NULL,
  environment_id VARCHAR(64) NOT NULL,
  server_id VARCHAR(64) NOT NULL,
  service_version_id VARCHAR(64) NOT NULL,
  deploy_record_id VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL,
  UNIQUE KEY uniq_server_deployment_state (service_id, environment_id, server_id)
);

INSERT INTO server_deployment_states (
  id, service_id, environment_id, server_id, service_version_id, deploy_record_id, updated_at
)
SELECT
  id, service_id, environment_id, target_ref_id, service_version_id, deploy_record_id, updated_at
FROM deployment_states
WHERE target_type IN ('server', 'server_group_member');

ALTER TABLE deployment_targets
  ADD COLUMN target_type VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN target_ref_id VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN script_path TEXT NOT NULL,
  ADD COLUMN working_dir TEXT NOT NULL,
  ADD COLUMN env_vars TEXT NOT NULL;

UPDATE deployment_targets dt
JOIN ssh_deployment_targets ssh ON ssh.deployment_target_id = dt.id
SET
  dt.target_type = ssh.target_type,
  dt.target_ref_id = ssh.target_ref_id,
  dt.script_path = ssh.script_path,
  dt.working_dir = ssh.working_dir,
  dt.env_vars = ssh.env_vars;

ALTER TABLE deploy_records RENAME COLUMN total_targets TO total_servers;
ALTER TABLE deploy_records RENAME COLUMN success_targets TO success_servers;
ALTER TABLE deploy_records RENAME COLUMN failed_targets TO failed_servers;
ALTER TABLE deploy_records RENAME COLUMN skipped_targets TO skipped_servers;

DROP TABLE deployment_states;
DROP TABLE deploy_target_logs;
DROP TABLE k8s_deployment_targets;
DROP TABLE k8s_clusters;
DROP TABLE ssh_deployment_targets;
