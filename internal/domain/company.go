package domain

import "time"

// Company is an employer, deduplicated by NormalizedName within a tenant.
//
// Normalization strips legal suffixes and punctuation so "Acme, Inc." and
// "acme" resolve to the same record. It is the anchor for the fuzzy match
// tier: two postings are only ever compared for similarity when their
// companies already agree.
type Company struct {
	ID       ID
	TenantID ID

	// Name is the display form, as first seen.
	Name string
	// NormalizedName is the dedupe key, unique per tenant.
	NormalizedName string

	Domain     string
	CareersURL string

	CreatedAt time.Time
	UpdatedAt time.Time
}
