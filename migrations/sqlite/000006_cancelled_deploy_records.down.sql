PRAGMA foreign_keys=off;

UPDATE deploy_records SET status = 'failed', error_summary = 'cancelled deploy record downgraded' WHERE status = 'cancelled';

ALTER TABLE deploy_records RENAME TO deploy_records_old;

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

INSERT INTO deploy_records (
  id, release_request_id, status, executor_type, target_snapshot, total_servers,
  success_servers, failed_servers, skipped_servers, worker_id, lease_expires_at,
  heartbeat_at, started_at, finished_at, error_summary, created_at, updated_at
)
SELECT
  id, release_request_id, status, executor_type, target_snapshot, total_servers,
  success_servers, failed_servers, skipped_servers, worker_id, lease_expires_at,
  heartbeat_at, started_at, finished_at, error_summary, created_at, updated_at
FROM deploy_records_old;

DROP TABLE deploy_records_old;

CREATE INDEX idx_deploy_records_status ON deploy_records(status);
CREATE INDEX idx_deploy_records_created_at ON deploy_records(created_at);

PRAGMA foreign_keys=on;
