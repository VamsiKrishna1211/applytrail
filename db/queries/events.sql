-- name: InsertApplicationEvent :one
INSERT INTO application_events (
    tenant_id, application_id, type, from_status, to_status, occurred_at, actor, payload
) VALUES (
    @tenant_id, @application_id, @type, @from_status, @to_status, @occurred_at, @actor, @payload
)
RETURNING *;

-- name: ListApplicationEvents :many
SELECT * FROM application_events
WHERE tenant_id = @tenant_id AND application_id = @application_id
ORDER BY occurred_at DESC, created_at DESC
LIMIT @row_limit;

-- Most recent transition into the application's current status: how long the
-- current stage has been running.
-- name: GetLatestStatusChange :one
SELECT * FROM application_events
WHERE tenant_id = @tenant_id
  AND application_id = @application_id
  AND type = 'status_changed'
ORDER BY occurred_at DESC, created_at DESC
LIMIT 1;
