package domain

import "time"

// PostingRef is a lightweight view of an existing posting plus the tenant's
// relationship to it. Every dedupe lookup returns this shape, whichever tier
// produced it, so the match engine handles one type instead of five.
//
// It carries the joined application status deliberately: the resurfacing
// policy needs it, and fetching it here avoids a second round trip per match.
type PostingRef struct {
	PostingID    ID
	CanonicalURL string

	Source          Source
	ExternalID      string
	Fingerprint     string
	DescriptionHash string
	NormalizedTitle string
	LocationBucket  string

	FirstSeenAt time.Time
	LastSeenAt  time.Time

	ApplicationID     *ID
	ApplicationStatus *Status

	// Similarity is the measured trigram score. Only the fuzzy tier sets it;
	// identifier-based tiers leave it zero because they did not measure
	// anything.
	Similarity float64
}

// EffectiveStatus returns the application status, defaulting to
// StatusDiscovered when a posting was recorded without one.
func (r PostingRef) EffectiveStatus() Status {
	if r.ApplicationStatus == nil {
		return StatusDiscovered
	}
	return *r.ApplicationStatus
}

// SourceKey is a (source, external id) pair: the ATS identity of a posting.
type SourceKey struct {
	Source     Source
	ExternalID string
}

// String renders the key in the "source|external_id" form the batch lookup
// query compares against.
func (k SourceKey) String() string { return string(k.Source) + "|" + k.ExternalID }
