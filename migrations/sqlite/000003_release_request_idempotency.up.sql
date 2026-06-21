CREATE UNIQUE INDEX idx_release_requests_idempotency_key
ON release_requests(idempotency_key)
WHERE idempotency_key IS NOT NULL;
