-- +goose Up

-- application_contacts had no tenant_id of its own: nothing in the schema
-- stopped an application in one tenant from being linked to a contact in
-- another. The table is empty in every deployment this migration has been
-- tested against, so the backfill is a formality, not a real repair.
ALTER TABLE application_contacts ADD COLUMN tenant_id uuid;
UPDATE application_contacts ac
SET tenant_id = a.tenant_id
FROM applications a
WHERE a.id = ac.application_id;
ALTER TABLE application_contacts ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE application_contacts
    ADD CONSTRAINT application_contacts_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE application_contacts DROP CONSTRAINT application_contacts_pkey;
ALTER TABLE application_contacts
    ADD CONSTRAINT application_contacts_pkey
    PRIMARY KEY (tenant_id, application_id, contact_id);

-- 'ghosted' is explicitly re-enterable (an employer can resurface after
-- months of silence), so a follow-up date set while ghosted must be able to
-- surface in the queue the same as any other engaged status. The old index
-- excluded it, which meant UpdateStatus stored a next_action_at that could
-- never be read back.
DROP INDEX applications_followup_idx;
CREATE INDEX applications_followup_idx
    ON applications (tenant_id, next_action_at)
    WHERE next_action_at IS NOT NULL
      AND status IN ('applied', 'screening', 'interview', 'offer', 'ghosted');

-- A posting with no identifiable title (Candidate.Validate permits a
-- url-only candidate, but recording one now requires a title — see the Go
-- change alongside this migration) fingerprints as company+""+location, a
-- value distinct postings can share. Making the index partial means two such
-- rows no longer collide on insert; the application-level guard is what
-- actually prevents them from being created in the first place.
DROP INDEX postings_tenant_fingerprint_key;
CREATE UNIQUE INDEX postings_tenant_fingerprint_key
    ON postings (tenant_id, fingerprint)
    WHERE normalized_title <> '';

-- The run audit folded "eligible again" into new_count, so a run's own
-- candidates_seen never equalled new_count + duplicate_count +
-- needs_review_count. Giving eligible-again its own column is what makes the
-- audit add up.
ALTER TABLE search_runs ADD COLUMN eligible_again_count integer NOT NULL DEFAULT 0;
ALTER TABLE search_runs ADD CONSTRAINT search_runs_eligible_again_non_negative
    CHECK (eligible_again_count >= 0);

-- +goose Down
ALTER TABLE search_runs DROP CONSTRAINT search_runs_eligible_again_non_negative;
ALTER TABLE search_runs DROP COLUMN eligible_again_count;

DROP INDEX postings_tenant_fingerprint_key;
CREATE UNIQUE INDEX postings_tenant_fingerprint_key
    ON postings (tenant_id, fingerprint);

DROP INDEX applications_followup_idx;
CREATE INDEX applications_followup_idx
    ON applications (tenant_id, next_action_at)
    WHERE next_action_at IS NOT NULL
      AND status IN ('applied', 'screening', 'interview', 'offer');

ALTER TABLE application_contacts DROP CONSTRAINT application_contacts_pkey;
ALTER TABLE application_contacts
    ADD CONSTRAINT application_contacts_pkey
    PRIMARY KEY (application_id, contact_id);
ALTER TABLE application_contacts DROP CONSTRAINT application_contacts_tenant_id_fkey;
ALTER TABLE application_contacts DROP COLUMN tenant_id;
