-- name: CreateTenant :one
INSERT INTO tenants (name, slug)
VALUES (@name, @slug)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = @id;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = @slug;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at;
