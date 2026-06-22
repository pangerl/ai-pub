CREATE TABLE service_accounts (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  owner_user_id VARCHAR(64),
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE TABLE api_keys (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  prefix VARCHAR(64) NOT NULL,
  key_hash VARCHAR(128) NOT NULL UNIQUE,
  owner_type VARCHAR(32) NOT NULL,
  owner_id VARCHAR(64) NOT NULL,
  scopes TEXT NOT NULL,
  expires_at VARCHAR(64),
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  last_used_at VARCHAR(64),
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE INDEX idx_api_keys_prefix ON api_keys(prefix);

CREATE TABLE services (
  id VARCHAR(64) PRIMARY KEY,
  project_id VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  slug VARCHAR(255) NOT NULL,
  description TEXT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL,
  UNIQUE KEY uniq_services_project_slug (project_id, slug)
);

CREATE TABLE service_versions (
  id VARCHAR(64) PRIMARY KEY,
  service_id VARCHAR(64) NOT NULL,
  version VARCHAR(255) NOT NULL,
  commit_sha VARCHAR(255) NOT NULL,
  artifact_url TEXT NOT NULL,
  source VARCHAR(32) NOT NULL,
  metadata TEXT NOT NULL,
  created_by_type VARCHAR(32) NOT NULL,
  created_by_id VARCHAR(64) NOT NULL,
  created_at VARCHAR(64) NOT NULL,
  UNIQUE KEY uniq_service_versions_service_version (service_id, version)
);

CREATE TABLE servers (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  host VARCHAR(255) NOT NULL,
  port INT NOT NULL DEFAULT 22,
  username VARCHAR(255) NOT NULL,
  auth_type VARCHAR(32) NOT NULL,
  credential_ref VARCHAR(64) NOT NULL,
  role VARCHAR(32) NOT NULL DEFAULT 'application',
  gateway_id VARCHAR(64) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  last_check_status VARCHAR(64) NOT NULL,
  last_check_at VARCHAR(64),
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE TABLE server_groups (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  description TEXT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE TABLE server_group_members (
  server_group_id VARCHAR(64) NOT NULL,
  server_id VARCHAR(64) NOT NULL,
  PRIMARY KEY (server_group_id, server_id)
);

CREATE TABLE deployment_targets (
  id VARCHAR(64) PRIMARY KEY,
  service_id VARCHAR(64) NOT NULL,
  environment_id VARCHAR(64) NOT NULL,
  executor_type VARCHAR(32) NOT NULL,
  target_type VARCHAR(32) NOT NULL,
  target_ref_id VARCHAR(64) NOT NULL,
  script_path TEXT NOT NULL,
  working_dir TEXT NOT NULL,
  env_vars TEXT NOT NULL,
  timeout_seconds INT NOT NULL DEFAULT 300,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE INDEX idx_deployment_targets_service_env ON deployment_targets(service_id, environment_id);

CREATE TABLE release_requests (
  id VARCHAR(64) PRIMARY KEY,
  project_id VARCHAR(64) NOT NULL,
  service_id VARCHAR(64) NOT NULL,
  environment_id VARCHAR(64) NOT NULL,
  service_version_id VARCHAR(64) NOT NULL,
  deployment_target_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  source VARCHAR(32) NOT NULL,
  idempotency_key VARCHAR(255),
  created_by_type VARCHAR(32) NOT NULL,
  created_by_id VARCHAR(64) NOT NULL,
  authorized_by_user_id VARCHAR(64),
  confirmed_by_user_id VARCHAR(64),
  confirmed_at VARCHAR(64),
  rejected_by_user_id VARCHAR(64),
  rejected_reason TEXT NOT NULL,
  rollback_of_id VARCHAR(64),
  summary_status VARCHAR(32) NOT NULL,
  summary_message TEXT NOT NULL,
  metadata TEXT NOT NULL,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE INDEX idx_release_requests_status ON release_requests(status);
CREATE INDEX idx_release_requests_service_env ON release_requests(service_id, environment_id);
CREATE INDEX idx_release_requests_created_at ON release_requests(created_at);

CREATE TABLE deploy_records (
  id VARCHAR(64) PRIMARY KEY,
  release_request_id VARCHAR(64) NOT NULL UNIQUE,
  status VARCHAR(32) NOT NULL,
  executor_type VARCHAR(32) NOT NULL,
  target_snapshot TEXT NOT NULL,
  total_servers INT NOT NULL DEFAULT 0,
  success_servers INT NOT NULL DEFAULT 0,
  failed_servers INT NOT NULL DEFAULT 0,
  skipped_servers INT NOT NULL DEFAULT 0,
  worker_id VARCHAR(255) NOT NULL,
  lease_expires_at VARCHAR(64),
  heartbeat_at VARCHAR(64),
  started_at VARCHAR(64),
  finished_at VARCHAR(64),
  error_summary TEXT NOT NULL,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE INDEX idx_deploy_records_status ON deploy_records(status);
CREATE INDEX idx_deploy_records_created_at ON deploy_records(created_at);

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

CREATE TABLE release_events (
  id VARCHAR(64) PRIMARY KEY,
  release_request_id VARCHAR(64),
  deploy_record_id VARCHAR(64),
  event_type VARCHAR(64) NOT NULL,
  actor_type VARCHAR(32) NOT NULL,
  actor_id VARCHAR(64) NOT NULL,
  authorized_user_id VARCHAR(64),
  api_key_id VARCHAR(64),
  source_ip VARCHAR(64) NOT NULL,
  message TEXT NOT NULL,
  metadata TEXT NOT NULL,
  created_at VARCHAR(64) NOT NULL
);

CREATE INDEX idx_release_events_release ON release_events(release_request_id, created_at);
CREATE INDEX idx_release_events_deploy ON release_events(deploy_record_id, created_at);

CREATE TABLE notification_configs (
  id VARCHAR(64) PRIMARY KEY,
  channel VARCHAR(32) NOT NULL,
  name VARCHAR(255) NOT NULL,
  webhook_url_enc TEXT NOT NULL,
  secret_enc TEXT NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);

CREATE TABLE notification_deliveries (
  id VARCHAR(64) PRIMARY KEY,
  config_id VARCHAR(64) NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  release_request_id VARCHAR(64),
  deploy_record_id VARCHAR(64),
  status VARCHAR(32) NOT NULL,
  attempt_count INT NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL,
  sent_at VARCHAR(64),
  created_at VARCHAR(64) NOT NULL,
  updated_at VARCHAR(64) NOT NULL
);
