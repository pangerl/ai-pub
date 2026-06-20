PRAGMA foreign_keys=off;

ALTER TABLE release_events RENAME TO release_events_old;

CREATE TABLE release_events (
  id TEXT PRIMARY KEY,
  release_request_id TEXT,
  deploy_record_id TEXT,
  event_type TEXT NOT NULL,
  actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'ai_agent', 'service_account', 'system', 'api_key')),
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

INSERT INTO release_events (
  id, release_request_id, deploy_record_id, event_type, actor_type, actor_id,
  authorized_user_id, api_key_id, source_ip, message, metadata, created_at
)
SELECT
  id, release_request_id, deploy_record_id, event_type, actor_type, actor_id,
  authorized_user_id, api_key_id, source_ip, message, metadata, created_at
FROM release_events_old;

DROP TABLE release_events_old;

CREATE INDEX idx_release_events_release ON release_events(release_request_id, created_at);
CREATE INDEX idx_release_events_deploy ON release_events(deploy_record_id, created_at);

PRAGMA foreign_keys=on;
