package domain

import "time"

// MatchTier names which rule identified a duplicate. It is returned to callers
// so they can weigh a verdict: an ats_id match is a fact, a fuzzy match is an
// inference.
type MatchTier string

const (
	// TierNone means no existing posting matched.
	TierNone MatchTier = "none"
	// TierExactURL matched on the canonical URL.
	TierExactURL MatchTier = "exact_url"
	// TierATSID matched on (source, external id) parsed from the URL.
	TierATSID MatchTier = "ats_id"
	// TierFingerprint matched on normalized company, title, and location.
	TierFingerprint MatchTier = "fingerprint"
	// TierContentHash matched on the description hash, catching the same job
	// cross-posted under a different title.
	TierContentHash MatchTier = "content_hash"
	// TierFuzzy matched on title similarity within the same company.
	TierFuzzy MatchTier = "fuzzy"
)

// Verdict is the recommendation returned for a candidate.
type Verdict string

const (
	// VerdictNew means nothing matched; the candidate is worth pursuing.
	VerdictNew Verdict = "new"
	// VerdictDuplicate means a match exists and policy says do not resurface.
	VerdictDuplicate Verdict = "duplicate"
	// VerdictEligibleAgain means a match exists but enough time has passed
	// that it is worth another look.
	VerdictEligibleAgain Verdict = "eligible_again"
	// VerdictNeedsReview means the match was close but not conclusive. The
	// service refuses to guess; the caller decides.
	VerdictNeedsReview Verdict = "needs_review"
)

// Match is what the dedupe engine found for one candidate.
type Match struct {
	Tier MatchTier
	// Confidence is 1.0 for identifier-based tiers and the measured similarity
	// for the fuzzy tier.
	Confidence float64
	PostingID  *ID
}

// Found reports whether any existing posting matched.
func (m Match) Found() bool { return m.Tier != TierNone && m.PostingID != nil }

// FilterResult is the per-candidate answer returned by both the REST batch
// dedupe endpoint and the filter_new_postings MCP tool.
//
// It reports the facts alongside the verdict deliberately: an agent that
// disagrees with the recommendation can still act on LastSeenAt and
// ApplicationStatus.
type FilterResult struct {
	// Index ties the result back to its position in the request, so callers
	// need no correlation ID.
	Index int

	Verdict    Verdict
	Tier       MatchTier
	Confidence float64

	PostingID         *ID
	ApplicationID     *ID
	ApplicationStatus *Status

	FirstSeenAt *time.Time
	LastSeenAt  *time.Time
	// DaysSince counts days since LastSeenAt, precomputed so the caller does
	// not have to do date arithmetic.
	DaysSince *int

	// Reason is a short human-readable explanation, meant to be read by a
	// model or shown in a log line.
	Reason string
}
