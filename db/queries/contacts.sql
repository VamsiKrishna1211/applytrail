-- Upsert keyed on email when present; a contact without one is always an
-- insert, because there is no reliable natural key.
-- name: UpsertContactByEmail :one
INSERT INTO contacts (tenant_id, company_id, name, role, email, linkedin_url, notes, last_contact_at)
VALUES (@tenant_id, @company_id, @name, @role, @email, @linkedin_url, @notes, @last_contact_at)
ON CONFLICT (tenant_id, lower(email)) WHERE email <> '' DO UPDATE
SET company_id      = COALESCE(EXCLUDED.company_id, contacts.company_id),
    name            = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE contacts.name END,
    role            = CASE WHEN EXCLUDED.role <> '' THEN EXCLUDED.role ELSE contacts.role END,
    linkedin_url    = CASE WHEN EXCLUDED.linkedin_url <> '' THEN EXCLUDED.linkedin_url ELSE contacts.linkedin_url END,
    notes           = CASE WHEN EXCLUDED.notes <> '' THEN EXCLUDED.notes ELSE contacts.notes END,
    last_contact_at = GREATEST(contacts.last_contact_at, EXCLUDED.last_contact_at),
    updated_at      = now()
RETURNING *;

-- name: GetContact :one
SELECT * FROM contacts WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListContacts :many
SELECT * FROM contacts
WHERE tenant_id = @tenant_id
  AND (sqlc.narg('company_id')::uuid IS NULL OR company_id = sqlc.narg('company_id')::uuid)
ORDER BY updated_at DESC
LIMIT @row_limit;

-- name: LinkContactToApplication :exec
INSERT INTO application_contacts (application_id, contact_id)
VALUES (@application_id, @contact_id)
ON CONFLICT DO NOTHING;

-- name: ListContactsForApplication :many
SELECT c.* FROM contacts c
JOIN application_contacts ac ON ac.contact_id = c.id
WHERE ac.application_id = @application_id AND c.tenant_id = @tenant_id
ORDER BY c.updated_at DESC;
