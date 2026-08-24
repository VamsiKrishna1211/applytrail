-- Idempotent by (tenant, normalized_name): concurrent ingests of the same
-- employer converge on one row instead of racing.
-- name: UpsertCompany :one
INSERT INTO companies (tenant_id, name, normalized_name, domain, careers_url)
VALUES (@tenant_id, @name, @normalized_name, @domain, @careers_url)
ON CONFLICT (tenant_id, normalized_name) DO UPDATE
SET domain      = CASE WHEN companies.domain = ''      THEN EXCLUDED.domain      ELSE companies.domain END,
    careers_url = CASE WHEN companies.careers_url = '' THEN EXCLUDED.careers_url ELSE companies.careers_url END,
    updated_at  = now()
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetCompanyByNormalizedName :one
SELECT * FROM companies
WHERE tenant_id = @tenant_id AND normalized_name = @normalized_name;

-- name: ListCompanies :many
SELECT * FROM companies
WHERE tenant_id = @tenant_id
ORDER BY normalized_name;
