-- +goose Up

CREATE TABLE search_profiles (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    name        text    NOT NULL,
    description text    NOT NULL DEFAULT '',
    active      boolean NOT NULL DEFAULT true,

    keywords  text[] NOT NULL DEFAULT '{}',
    titles    text[] NOT NULL DEFAULT '{}',
    locations text[] NOT NULL DEFAULT '{}',

    remote_only      boolean NOT NULL DEFAULT false,
    min_compensation bigint,
    currency         text    NOT NULL DEFAULT '',
    seniority        text[]  NOT NULL DEFAULT '{}',

    company_allowlist text[] NOT NULL DEFAULT '{}',
    company_blocklist text[] NOT NULL DEFAULT '{}',
    exclude_keywords  text[] NOT NULL DEFAULT '{}',

    -- Per-status cooldowns in days, e.g. {"screened_out": 90, "applied": -1}.
    -- Statuses absent here fall back to the built-in default policy, so a
    -- profile only records its deviations.
    resurface_policy jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT search_profiles_name_not_empty CHECK (name <> ''),
    CONSTRAINT search_profiles_min_compensation_non_negative
        CHECK (min_compensation IS NULL OR min_compensation >= 0)
);

CREATE UNIQUE INDEX search_profiles_tenant_name_key
    ON search_profiles (tenant_id, name);

CREATE TABLE search_runs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    profile_id uuid REFERENCES search_profiles (id) ON DELETE SET NULL,

    source text NOT NULL DEFAULT '',
    query  text NOT NULL DEFAULT '',

    candidates_seen    integer NOT NULL DEFAULT 0,
    new_count          integer NOT NULL DEFAULT 0,
    duplicate_count    integer NOT NULL DEFAULT 0,
    needs_review_count integer NOT NULL DEFAULT 0,

    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    notes       text NOT NULL DEFAULT '',

    CONSTRAINT search_runs_counts_non_negative CHECK (
        candidates_seen >= 0 AND new_count >= 0
        AND duplicate_count >= 0 AND needs_review_count >= 0
    )
);

COMMENT ON TABLE search_runs IS
    'Audit of each execution: answers "did the routine run today, and did it find anything?"';

CREATE INDEX search_runs_tenant_started_idx ON search_runs (tenant_id, started_at DESC);
CREATE INDEX search_runs_profile_idx ON search_runs (profile_id, started_at DESC);

-- +goose Down
DROP TABLE search_runs;
DROP TABLE search_profiles;
