-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, prefix, hash, label, scopes, expires_at)
VALUES (@tenant_id, @prefix, @hash, @label, @scopes, @expires_at)
RETURNING *;

-- Authentication path: the prefix is unique and indexed, so verification
-- hashes exactly one candidate rather than every key in the table.
-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys WHERE prefix = @prefix;

-- name: ListAPIKeys :many
SELECT * FROM api_keys
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = @used_at WHERE id = @id;

-- name: RevokeAPIKey :one
UPDATE api_keys
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;
