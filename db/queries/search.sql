-- name: CreateSearchProfile :one
INSERT INTO search_profiles (
    tenant_id, name, description, active, keywords, titles, locations,
    remote_only, min_compensation, currency, seniority,
    company_allowlist, company_blocklist, exclude_keywords, resurface_policy
) VALUES (
    @tenant_id, @name, @description, @active, @keywords, @titles, @locations,
    @remote_only, @min_compensation, @currency, @seniority,
    @company_allowlist, @company_blocklist, @exclude_keywords, @resurface_policy
)
RETURNING *;

-- name: GetSearchProfile :one
SELECT * FROM search_profiles WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetSearchProfileByName :one
SELECT * FROM search_profiles WHERE tenant_id = @tenant_id AND name = @name;

-- name: ListSearchProfiles :many
SELECT * FROM search_profiles
WHERE tenant_id = @tenant_id
  AND (@active_only::boolean = false OR active = true)
ORDER BY name;

-- name: UpdateSearchProfile :one
UPDATE search_profiles
SET description       = COALESCE(sqlc.narg('description')::text, description),
    active            = COALESCE(sqlc.narg('active')::boolean, active),
    keywords          = COALESCE(sqlc.narg('keywords')::text[], keywords),
    titles            = COALESCE(sqlc.narg('titles')::text[], titles),
    locations         = COALESCE(sqlc.narg('locations')::text[], locations),
    remote_only       = COALESCE(sqlc.narg('remote_only')::boolean, remote_only),
    min_compensation  = COALESCE(sqlc.narg('min_compensation')::bigint, min_compensation),
    currency          = COALESCE(sqlc.narg('currency')::text, currency),
    seniority         = COALESCE(sqlc.narg('seniority')::text[], seniority),
    company_allowlist = COALESCE(sqlc.narg('company_allowlist')::text[], company_allowlist),
    company_blocklist = COALESCE(sqlc.narg('company_blocklist')::text[], company_blocklist),
    exclude_keywords  = COALESCE(sqlc.narg('exclude_keywords')::text[], exclude_keywords),
    resurface_policy  = COALESCE(sqlc.narg('resurface_policy')::jsonb, resurface_policy),
    updated_at        = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: CreateSearchRun :one
INSERT INTO search_runs (tenant_id, profile_id, source, query, notes)
VALUES (@tenant_id, @profile_id, @source, @query, @notes)
RETURNING *;

-- Counts accumulate across calls so a long routine can report incrementally
-- rather than buffering every result until the run ends.
-- name: RecordSearchRunProgress :one
UPDATE search_runs
SET candidates_seen    = candidates_seen + @candidates_seen,
    new_count          = new_count + @new_count,
    duplicate_count    = duplicate_count + @duplicate_count,
    needs_review_count = needs_review_count + @needs_review_count
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: FinishSearchRun :one
UPDATE search_runs
SET finished_at = COALESCE(finished_at, @finished_at),
    notes       = CASE WHEN @notes::text <> '' THEN @notes ELSE notes END
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: ListSearchRuns :many
SELECT * FROM search_runs
WHERE tenant_id = @tenant_id
  AND (sqlc.narg('profile_id')::uuid IS NULL OR profile_id = sqlc.narg('profile_id')::uuid)
  AND (sqlc.narg('since')::timestamptz IS NULL OR started_at >= sqlc.narg('since')::timestamptz)
ORDER BY started_at DESC
LIMIT @row_limit;
