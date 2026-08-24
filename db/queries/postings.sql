-- Dedupe lookups are batched: one round trip per match tier for the whole
-- candidate set, rather than one per candidate. Each returns the joined
-- application status so the resurfacing policy can be applied without a
-- second query.

-- name: FindPostingsByCanonicalURLs :many
SELECT p.id, p.canonical_url, p.source, p.external_id, p.fingerprint,
       p.description_hash, p.first_seen_at, p.last_seen_at,
       a.id AS application_id, a.status AS application_status
FROM postings p
LEFT JOIN applications a ON a.posting_id = p.id
WHERE p.tenant_id = @tenant_id
  AND p.canonical_url = ANY (@canonical_urls::text[]);

-- Matches on ATS identity. Source and external ID are compared as one
-- "source|external_id" key so the whole candidate set is a single array
-- parameter; the expression index in migration 00002 serves it directly.
-- name: FindPostingsByExternalIDs :many
SELECT p.id, p.canonical_url, p.source, p.external_id, p.fingerprint,
       p.description_hash, p.first_seen_at, p.last_seen_at,
       a.id AS application_id, a.status AS application_status
FROM postings p
LEFT JOIN applications a ON a.posting_id = p.id
WHERE p.tenant_id = @tenant_id
  AND p.external_id <> ''
  AND (p.source || '|' || p.external_id) = ANY (@source_external_ids::text[]);

-- name: FindPostingsByFingerprints :many
SELECT p.id, p.canonical_url, p.source, p.external_id, p.fingerprint,
       p.description_hash, p.first_seen_at, p.last_seen_at,
       a.id AS application_id, a.status AS application_status
FROM postings p
LEFT JOIN applications a ON a.posting_id = p.id
WHERE p.tenant_id = @tenant_id
  AND p.fingerprint = ANY (@fingerprints::text[]);

-- name: FindPostingsByDescriptionHashes :many
SELECT p.id, p.canonical_url, p.source, p.external_id, p.fingerprint,
       p.description_hash, p.first_seen_at, p.last_seen_at,
       a.id AS application_id, a.status AS application_status
FROM postings p
LEFT JOIN applications a ON a.posting_id = p.id
WHERE p.tenant_id = @tenant_id
  AND p.description_hash <> ''
  AND p.description_hash = ANY (@description_hashes::text[]);

-- Fuzzy tier. The `%` operator uses the pg_trgm GIN index to shortlist
-- plausible titles; exact scoring happens in Go, which owns the thresholds.
-- Scoped to one company because a title alone is far too weak a signal.
-- name: FindFuzzyPostingCandidates :many
SELECT p.id, p.canonical_url, p.normalized_title, p.location_bucket,
       p.first_seen_at, p.last_seen_at,
       a.id AS application_id, a.status AS application_status,
       similarity(p.normalized_title, @normalized_title::text) AS score
FROM postings p
LEFT JOIN applications a ON a.posting_id = p.id
WHERE p.tenant_id = @tenant_id
  AND p.company_id = @company_id
  AND p.normalized_title % @normalized_title::text
ORDER BY score DESC
LIMIT @row_limit;

-- name: InsertPosting :one
INSERT INTO postings (
    tenant_id, company_id, source, external_id, canonical_url, raw_url,
    title, normalized_title, location_raw, location_bucket, remote_type,
    seniority, comp_min, comp_max, comp_currency, visa_sponsorship, tech_tags,
    description_hash, fingerprint, posted_at, first_seen_at, last_seen_at, raw
) VALUES (
    @tenant_id, @company_id, @source, @external_id, @canonical_url, @raw_url,
    @title, @normalized_title, @location_raw, @location_bucket, @remote_type,
    @seniority, @comp_min, @comp_max, @comp_currency, @visa_sponsorship, @tech_tags,
    @description_hash, @fingerprint, @posted_at, @seen_at, @seen_at, @raw
)
RETURNING *;

-- name: GetPosting :one
SELECT * FROM postings WHERE id = @id AND tenant_id = @tenant_id;

-- Records that an existing posting was seen again. Only ever moves last_seen_at
-- forward, so an out-of-order ingest cannot rewind it.
-- name: TouchPostingSeen :one
UPDATE postings
SET last_seen_at = GREATEST(last_seen_at, @seen_at),
    updated_at   = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: ClosePosting :exec
UPDATE postings
SET closed_at = COALESCE(closed_at, @closed_at), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;
