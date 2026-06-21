CREATE TABLE service_accounts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  owner_user_id TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (owner_user_id) REFERENCES users(id)
);

CREATE TABLE api_keys (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL UNIQUE,
  owner_type TEXT NOT NULL CHECK (owner_type IN ('user', 'service_account')),
  owner_id TEXT NOT NULL,
  scopes TEXT NOT NULL DEFAULT '[]',
  expires_at TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_used_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_api_keys_prefix ON api_keys(prefix);

CREATE TABLE services (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  UNIQUE (project_id, slug)
);

CREATE TABLE service_versions (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  version TEXT NOT NULL,
  commit_sha TEXT NOT NULL DEFAULT '',
  artifact_url TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL CHECK (source IN ('manual', 'ci', 'api')),
  metadata TEXT NOT NULL DEFAULT '{}',
  created_by_type TEXT NOT NULL DEFAULT 'user',
  created_by_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id),
  UNIQUE (service_id, version)
);

CREATE TABLE servers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  username TEXT NOT NULL,
  auth_type TEXT NOT NULL CHECK (auth_type IN ('password', 'private_key', 'none')),
  credential_ref TEXT NOT NULL DEFAULT '',
  gateway_id TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_check_status TEXT NOT NULL DEFAULT '',
  last_check_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE server_groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE server_group_members (
  server_group_id TEXT NOT NULL,
  server_id TEXT NOT NULL,
  PRIMARY KEY (server_group_id, server_id),
  FOREIGN KEY (server_group_id) REFERENCES server_groups(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE deployment_targets (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  executor_type TEXT NOT NULL CHECK (executor_type IN ('mock', 'ssh')),
  target_type TEXT NOT NULL CHECK (target_type IN ('server', 'server_group')),
  target_ref_id TEXT NOT NULL,
  script_path TEXT NOT NULL DEFAULT '',
  working_dir TEXT NOT NULL DEFAULT '',
  env_vars TEXT NOT NULL DEFAULT '{}',
  timeout_seconds INTEGER NOT NULL DEFAULT 300,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services(id),
  FOREIGN KEY (environment_id) REFERENCES environments(id)
);

CREATE INDEX idx_deployment_targets_service_env ON deployment_targets(service_id, environment_id);

CREATE TABLE release_requests (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  service_id TEXT NOT NULL,
  environment_id TEXT NOT NULL,
  service_version_id TEXT NOT NULL,
  deployment_target_id TEXT NOT NULL,
  status TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('web', 'api', 'ci', 'ai_agent')),
  idempotency_key TEXT,
  created_by_type TEXT NOT NULL,
  created_by_id TEXT NOT NULL,
  authorized_by_user_id TEXT,
  confirmed_by_user_id TEXT,
  confirmed_at TEXT,
  rejected_by_user_id TEXT,
  rejected_reason TEXT NOT NULL DEFAULT '',
  rollback_of_id TEXT,
  summary_status TEXT NOT NULL DEFAULT '',
  summary_message TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id),
  FOREIGN KEY (service_id) REFERENCES services(id),
  FOREIGN KEY (environment_id) REFERENCES environments(id),
  FOREIGN KEY (service_version_id) REFERENCES service_versions(id),
  FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id)
);

CREATE INDEX idx_release_requests_status ON release_requests(status);
CREATE INDEX idx_release_requests_service_env ON release_requests(service_id, environment_id);
CREATE INDEX idx_release_requests_created_at ON release_requests(created_at);

CREATE TABLE deploy_records (
  id TEXT PRIMARY KEY,
  release_request_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed', 'partial')),
  executor_type TEXT NOT NULL,
  target_snapshot TEXT NOT NULL DEFAULT '{}',
  total_servers INTEGER NOT NULL DEFAULT 0,
  success_servers INTEGER NOT NULL DEFAULT 0,
  failed_servers INTEGER NOT NULL DEFAULT 0,
  skipped_servers INTEGER NOT NULL DEFAULT 0,
  worker_id TEXT NOT NULL DEFAULT '',
  lease_expires_at TEXT,
  heartbeat_at TEXT,
  started_at TEXT,
  finished_at TEXT,
  error_summary TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (release_request_id) REFERENCES release_requests(id)
);

CREATE INDEX idx_deploy_records_status ON deploy_records(status);
CREATE INDEX idx_deploy_records_created_at ON deploy_records(created_at);

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

CREATE TABLE release_events (
  id TEXT PRIMARY KEY,
  release_request_id TEXT,
  deploy_record_id TEXT,
  event_type TEXT NOT NULL,
  actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'ai_agent', 'service_account', 'system')),
  actor_id TEXT NOT NULL DEFAULT '',
  authorized_user_id TEXT,
  api_key_id TEXT,
  source_ip TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  FOREIGN KEY (release_request_id) REFERENCES release_requests(id),
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id)
);

CREATE INDEX idx_release_events_release ON release_events(release_request_id, created_at);
CREATE INDEX idx_release_events_deploy ON release_events(deploy_record_id, created_at);

CREATE TABLE notification_configs (
  id TEXT PRIMARY KEY,
  channel TEXT NOT NULL CHECK (channel IN ('wecom_robot')),
  name TEXT NOT NULL,
  webhook_url_enc TEXT NOT NULL,
  secret_enc TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE notification_deliveries (
  id TEXT PRIMARY KEY,
  config_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  release_request_id TEXT,
  deploy_record_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  sent_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (config_id) REFERENCES notification_configs(id),
  FOREIGN KEY (release_request_id) REFERENCES release_requests(id),
  FOREIGN KEY (deploy_record_id) REFERENCES deploy_records(id)
);
