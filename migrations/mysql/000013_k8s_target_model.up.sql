-- Kubernetes Deployment executor target model.
-- MVP data may be rebuilt, but this migration preserves old SSH rows where possible.

CREATE TABLE ssh_deployment_targets (
  deployment_target_id VARCHAR(64) PRIMARY KEY,
  target_type VARCHAR(32) NOT NULL,
  target_ref_id VARCHAR(64) NOT NULL,
  script_path TEXT NOT NULL,
  working_dir TEXT NOT NULL,
  env_vars TEXT NOT NULL,
  CONSTRAINT fk_ssh_deployment_targets_target
    FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO ssh_deployment_targets (
  deployment_target_id, target_type, target_ref_id, script_path, working_dir, env_vars
)
SELECT id, target_type, target_ref_id, script_path, working_dir, env_vars
FROM deployment_targets
WHERE executor_type = 'ssh';

CREATE TABLE k8s_clusters (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL UNIQUE,
  credential_ref VARCHAR(64) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE k8s_deployment_targets (
  deployment_target_id VARCHAR(64) PRIMARY KEY,
  cluster_id VARCHAR(64) NOT NULL,
  namespace VARCHAR(255) NOT NULL,
  deployment_name VARCHAR(255) NOT NULL,
  container_name VARCHAR(255) NOT NULL,
  CONSTRAINT fk_k8s_deployment_targets_target
    FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id),
  CONSTRAINT fk_k8s_deployment_targets_cluster
    FOREIGN KEY (cluster_id) REFERENCES k8s_clusters(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE deploy_target_logs (
  id VARCHAR(64) PRIMARY KEY,
  deploy_record_id VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_ref_id VARCHAR(255) NOT NULL,
  target_name VARCHAR(255) NOT NULL,
  status VARCHAR(32) NOT NULL,
  exit_code INT,
  started_at VARCHAR(64),
  finished_at VARCHAR(64),
  duration_ms INT NOT NULL DEFAULT 0,
  log_output MEDIUMTEXT NOT NULL,
  error_code VARCHAR(64) NOT NULL,
  error_message TEXT NOT NULL,
  CONSTRAINT fk_deploy_target_logs_record
    FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

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
  id VARCHAR(64) PRIMARY KEY,
  service_id VARCHAR(64) NOT NULL,
  environment_id VARCHAR(64) NOT NULL,
  deployment_target_id VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_ref_id VARCHAR(255) NOT NULL,
  service_version_id VARCHAR(64) NOT NULL,
  deploy_record_id VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL,
  UNIQUE KEY uniq_deployment_state (
    service_id, environment_id, deployment_target_id, target_type, target_ref_id
  ),
  CONSTRAINT fk_deployment_states_target
    FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id),
  CONSTRAINT fk_deployment_states_record
    FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

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

ALTER TABLE deployment_targets
  DROP COLUMN target_type,
  DROP COLUMN target_ref_id,
  DROP COLUMN script_path,
  DROP COLUMN working_dir,
  DROP COLUMN env_vars;

DROP TABLE server_deployment_states;
DROP TABLE server_deploy_logs;
