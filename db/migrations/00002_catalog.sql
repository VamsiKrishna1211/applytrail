-- +goose Up

CREATE TABLE companies (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    name            text NOT NULL,
    -- Legal suffixes and punctuation stripped. This is the dedupe key, and the
    -- anchor for fuzzy title matching: titles are only ever compared within a
    -- single normalized company.
    normalized_name text NOT NULL,

    domain      text NOT NULL DEFAULT '',
    careers_url text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT companies_normalized_name_not_empty CHECK (normalized_name <> '')
);

CREATE UNIQUE INDEX companies_tenant_normalized_name_key
    ON companies (tenant_id, normalized_name);

CREATE TABLE postings (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id)   ON DELETE CASCADE,
    company_id uuid        NOT NULL REFERENCES companies (id) ON DELETE CASCADE,

    source      text NOT NULL DEFAULT '',
    -- Job ID parsed out of the URL by the ATS extractor, when recognisable.
    external_id text NOT NULL DEFAULT '',

    -- Tracking parameters stripped, components normalized.
    canonical_url text NOT NULL DEFAULT '',
    raw_url       text NOT NULL DEFAULT '',

    title            text NOT NULL,
    -- Abbreviations expanded, level markers stripped. Feeds the fingerprint
    -- and the trigram index below.
    normalized_title text NOT NULL,

    location_raw    text NOT NULL DEFAULT '',
    location_bucket text NOT NULL DEFAULT '',
    remote_type     text NOT NULL DEFAULT '',
    seniority       text NOT NULL DEFAULT '',

    comp_min      bigint,
    comp_max      bigint,
    comp_currency text NOT NULL DEFAULT '',

    visa_sponsorship boolean,
    tech_tags        text[] NOT NULL DEFAULT '{}',

    -- Catches the same job cross-posted under a different title.
    description_hash text NOT NULL DEFAULT '',
    -- sha256(normalized company | normalized title | location bucket).
    fingerprint      text NOT NULL,

    posted_at     timestamptz,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    closed_at     timestamptz,

    -- Whatever the caller sent, kept so a future extractor can backfill
    -- columns without re-fetching the posting.
    raw jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT postings_remote_type_known
        CHECK (remote_type IN ('', 'remote', 'hybrid', 'onsite')),
    CONSTRAINT postings_comp_range
        CHECK (comp_min IS NULL OR comp_max IS NULL OR comp_min <= comp_max)
);

-- Match tier 1: exact canonical URL. Partial, because a posting recorded by
-- title alone legitimately has no URL.
CREATE UNIQUE INDEX postings_tenant_canonical_url_key
    ON postings (tenant_id, canonical_url)
    WHERE canonical_url <> '';

-- Match tier 2: ATS identity.
CREATE UNIQUE INDEX postings_tenant_source_external_id_key
    ON postings (tenant_id, source, external_id)
    WHERE external_id <> '';

-- Batch dedupe compares source and external ID as one "source|external_id"
-- key, so the whole candidate set fits in a single array parameter. This
-- expression index serves that comparison; the composite index above does not.
CREATE INDEX postings_tenant_source_external_key_idx
    ON postings (tenant_id, (source || '|' || external_id))
    WHERE external_id <> '';

-- Match tier 3: normalized fingerprint.
CREATE UNIQUE INDEX postings_tenant_fingerprint_key
    ON postings (tenant_id, fingerprint);

-- Match tier 4: description hash. Not unique; two genuinely distinct roles can
-- share boilerplate text.
CREATE INDEX postings_tenant_description_hash_idx
    ON postings (tenant_id, description_hash)
    WHERE description_hash <> '';

-- Match tier 5: trigram shortlist for fuzzy title comparison.
CREATE INDEX postings_normalized_title_trgm_idx
    ON postings USING gin (normalized_title gin_trgm_ops);

CREATE INDEX postings_tenant_company_idx ON postings (tenant_id, company_id);
CREATE INDEX postings_tenant_last_seen_idx ON postings (tenant_id, last_seen_at DESC);

-- +goose Down
DROP TABLE postings;
DROP TABLE companies;
