-- +goose Up

CREATE TABLE applications (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id)  ON DELETE CASCADE,
    posting_id uuid        NOT NULL REFERENCES postings (id) ON DELETE CASCADE,

    status     text        NOT NULL DEFAULT 'discovered',
    applied_at timestamptz,

    -- Only the label, a URL, and a content hash. ApplyTrail never stores the
    -- document itself, which keeps a free-tier deployment free.
    resume_variant       text NOT NULL DEFAULT '',
    resume_url           text NOT NULL DEFAULT '',
    resume_hash          text NOT NULL DEFAULT '',
    cover_letter_variant text NOT NULL DEFAULT '',

    -- Drives the follow-up queue.
    next_action_at timestamptz,
    notes          text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT applications_status_known CHECK (status IN (
        'discovered', 'screened_out', 'queued', 'applied', 'screening',
        'interview', 'offer', 'accepted', 'rejected', 'ghosted', 'withdrawn'
    )),
    -- An applied application must say when. Enforced here rather than only in
    -- the service so a manual SQL fix cannot corrupt the funnel report.
    CONSTRAINT applications_applied_at_present CHECK (
        status NOT IN ('applied', 'screening', 'interview', 'offer', 'accepted')
        OR applied_at IS NOT NULL
    )
);

-- One application per posting: the pipeline tracks a relationship, and a
-- tenant has exactly one relationship with a given job.
CREATE UNIQUE INDEX applications_tenant_posting_key
    ON applications (tenant_id, posting_id);

CREATE INDEX applications_tenant_status_idx ON applications (tenant_id, status);

-- Serves the follow-up queue: due items, soonest first.
CREATE INDEX applications_followup_idx
    ON applications (tenant_id, next_action_at)
    WHERE next_action_at IS NOT NULL
      AND status IN ('applied', 'screening', 'interview', 'offer');

CREATE TABLE application_events (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants (id)      ON DELETE CASCADE,
    application_id uuid NOT NULL REFERENCES applications (id) ON DELETE CASCADE,

    type        text NOT NULL,
    from_status text,
    to_status   text,

    occurred_at timestamptz NOT NULL DEFAULT now(),
    -- Who caused this: an API key label, 'system', or a human name.
    actor       text  NOT NULL DEFAULT '',
    payload     jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT application_events_type_known CHECK (type IN (
        'created', 'status_changed', 'note_added', 'contact_logged',
        'follow_up_sent', 'rediscovered'
    )),
    CONSTRAINT application_events_status_change_complete CHECK (
        type <> 'status_changed' OR to_status IS NOT NULL
    )
);

COMMENT ON TABLE application_events IS
    'Append-only timeline. Never updated or deleted: this is what lets a caller '
    'reason about how long a stage has been running.';

CREATE INDEX application_events_timeline_idx
    ON application_events (application_id, occurred_at DESC);
CREATE INDEX application_events_tenant_occurred_idx
    ON application_events (tenant_id, occurred_at DESC);

CREATE TABLE contacts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    -- Contacts hang off a company rather than an application, because one
    -- recruiter usually handles several roles.
    company_id uuid REFERENCES companies (id) ON DELETE SET NULL,

    name         text NOT NULL DEFAULT '',
    role         text NOT NULL DEFAULT '',
    email        text NOT NULL DEFAULT '',
    linkedin_url text NOT NULL DEFAULT '',
    notes        text NOT NULL DEFAULT '',

    last_contact_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT contacts_identifiable CHECK (name <> '' OR email <> '')
);

-- Email is the natural upsert key when present.
CREATE UNIQUE INDEX contacts_tenant_email_key
    ON contacts (tenant_id, lower(email))
    WHERE email <> '';

CREATE INDEX contacts_tenant_company_idx ON contacts (tenant_id, company_id);

CREATE TABLE application_contacts (
    application_id uuid NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    contact_id     uuid NOT NULL REFERENCES contacts (id)     ON DELETE CASCADE,
    created_at     timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (application_id, contact_id)
);

CREATE INDEX application_contacts_contact_idx ON application_contacts (contact_id);

-- +goose Down
DROP TABLE application_contacts;
DROP TABLE contacts;
DROP TABLE application_events;
DROP TABLE applications;
