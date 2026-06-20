CREATE UNIQUE INDEX idx_release_requests_idempotency_key ON release_requests(idempotency_key);

INSERT INTO release_policies (
  id,
  scope_type,
  scope_id,
  confirm_mode,
  manual_freeze_enabled,
  ssh_realtime_check_required,
  created_at,
  updated_at
) VALUES (
  'policy_system_default',
  'system',
  '',
  'self_confirm',
  0,
  0,
  UTC_TIMESTAMP(3),
  UTC_TIMESTAMP(3)
);
