-- name: InsertApplication :one
INSERT INTO applications (
    tenant_id, posting_id, status, applied_at, resume_variant, resume_url,
    resume_hash, cover_letter_variant, next_action_at, notes
) VALUES (
    @tenant_id, @posting_id, @status, @applied_at, @resume_variant, @resume_url,
    @resume_hash, @cover_letter_variant, @next_action_at, @notes
)
ON CONFLICT (tenant_id, posting_id) DO NOTHING
RETURNING *;

-- name: GetApplication :one
SELECT * FROM applications WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetApplicationByPosting :one
SELECT * FROM applications WHERE posting_id = @posting_id AND tenant_id = @tenant_id;

-- Applies a status transition. The from_status guard makes this a compare-and-
-- swap: a concurrent transition loses instead of silently overwriting.
-- name: UpdateApplicationStatus :one
UPDATE applications
SET status         = @to_status,
    applied_at     = COALESCE(sqlc.narg('applied_at')::timestamptz, applied_at),
    next_action_at = CASE WHEN @clear_next_action::boolean THEN NULL
                          ELSE COALESCE(sqlc.narg('next_action_at')::timestamptz, next_action_at) END,
    updated_at     = now()
WHERE id = @id AND tenant_id = @tenant_id AND status = @from_status
RETURNING *;

-- Partial update: every field falls back to its current value when the
-- corresponding argument is NULL.
-- name: UpdateApplicationFields :one
UPDATE applications
SET resume_variant       = COALESCE(sqlc.narg('resume_variant')::text, resume_variant),
    resume_url           = COALESCE(sqlc.narg('resume_url')::text, resume_url),
    resume_hash          = COALESCE(sqlc.narg('resume_hash')::text, resume_hash),
    cover_letter_variant = COALESCE(sqlc.narg('cover_letter_variant')::text, cover_letter_variant),
    next_action_at       = CASE WHEN @clear_next_action::boolean THEN NULL
                                ELSE COALESCE(sqlc.narg('next_action_at')::timestamptz, next_action_at) END,
    notes                = COALESCE(sqlc.narg('notes')::text, notes),
    updated_at           = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- Keyset pagination on (created_at, id): stable under concurrent inserts,
-- unlike OFFSET. Filters are optional; a NULL argument disables its clause.
--
-- Both joins are tenant-pinned, not just the outer WHERE: the schema does not
-- forbid an applications row from referencing another tenant's posting (the
-- unique constraint is on (tenant_id, posting_id), not on posting_id alone),
-- so an unpinned join is a latent cross-tenant read.
-- name: ListApplications :many
SELECT a.*, p.title, p.canonical_url, p.location_bucket, c.name AS company_name
FROM applications a
JOIN postings  p ON p.id = a.posting_id AND p.tenant_id = a.tenant_id
JOIN companies c ON c.id = p.company_id AND c.tenant_id = a.tenant_id
WHERE a.tenant_id = @tenant_id
  AND (sqlc.narg('statuses')::text[] IS NULL
       OR a.status = ANY (sqlc.narg('statuses')::text[]))
  AND (sqlc.narg('company_normalized_name')::text IS NULL
       OR c.normalized_name = sqlc.narg('company_normalized_name')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL
       OR a.created_at >= sqlc.narg('since')::timestamptz)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
       OR (a.created_at, a.id)
          < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY a.created_at DESC, a.id DESC
LIMIT @row_limit;

-- The follow-up queue: engaged applications whose next action is due.
-- 'ghosted' is included even though it is not IsEngaged(): it is explicitly
-- re-enterable (an employer can resurface after months of silence) and
-- UpdateStatus only clears next_action_at on a *terminal* status, so a
-- follow-up date set while ghosted must still be able to surface here.
-- name: ListFollowupsDue :many
SELECT a.*, p.title, p.canonical_url, p.location_bucket, c.name AS company_name
FROM applications a
JOIN postings  p ON p.id = a.posting_id AND p.tenant_id = a.tenant_id
JOIN companies c ON c.id = p.company_id AND c.tenant_id = a.tenant_id
WHERE a.tenant_id = @tenant_id
  AND a.next_action_at IS NOT NULL
  AND a.next_action_at <= @due_before
  AND a.status IN ('applied', 'screening', 'interview', 'offer', 'ghosted')
ORDER BY a.next_action_at, a.id
LIMIT @row_limit;

-- name: CountApplicationsByStatus :many
SELECT status, count(*) AS total
FROM applications
WHERE tenant_id = @tenant_id
GROUP BY status;

-- Response rate per resume variant: of the applications sent with each
-- variant, how many progressed past 'applied'.
-- name: CountResponsesByResumeVariant :many
SELECT resume_variant,
       count(*)                                                        AS sent,
       count(*) FILTER (WHERE status IN ('screening', 'interview', 'offer', 'accepted')) AS responded,
       count(*) FILTER (WHERE status = 'rejected')                     AS rejected,
       count(*) FILTER (WHERE status = 'ghosted')                      AS ghosted
FROM applications
WHERE tenant_id = @tenant_id AND applied_at IS NOT NULL
GROUP BY resume_variant
ORDER BY sent DESC;
