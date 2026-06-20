DELETE FROM release_policies WHERE id = 'policy_system_default';
DROP INDEX idx_release_requests_idempotency_key ON release_requests;
