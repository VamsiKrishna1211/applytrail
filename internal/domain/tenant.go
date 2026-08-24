package domain

import "time"

// Tenant owns every other record. ApplyTrail runs single-user today, but every
// table carries a tenant ID and every query filters on it, so multi-user is a
// deployment change rather than a schema migration.
type Tenant struct {
	ID   ID
	Name string
	Slug string

	CreatedAt time.Time
	UpdatedAt time.Time
}
