-- +goose Up

-- pg_trgm backs the fuzzy match tier: it lets Postgres shortlist similar
-- titles via a GIN index instead of scanning every posting for a company.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Status and scope columns are text with CHECK constraints rather than native
-- enums. Adding a value to a native enum is awkward to reverse, and this
-- schema is expected to grow statuses as people adopt it.

CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    slug       text        NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE tenants IS
    'Owner of every other row. Single-user deployments hold exactly one tenant; '
    'the column exists everywhere so multi-user is a deployment change, not a migration.';

CREATE TABLE api_keys (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,

    -- Non-secret leading portion, used to find the one row to verify against
    -- so authentication costs a single hash comparison.
    prefix     text        NOT NULL UNIQUE,
    -- argon2id hash of the full key. The key itself is never stored.
    hash       text        NOT NULL,

    label      text        NOT NULL DEFAULT '',
    scopes     text[]      NOT NULL,

    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT api_keys_scopes_not_empty CHECK (cardinality(scopes) > 0),
    CONSTRAINT api_keys_scopes_known CHECK (scopes <@ ARRAY['read', 'write', 'admin']::text[])
);

CREATE INDEX api_keys_tenant_idx ON api_keys (tenant_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE api_keys;
DROP TABLE tenants;
DROP EXTENSION IF EXISTS pg_trgm;
