package dedupe

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Confidence assigned to each identifier-based tier. These are not measured
// values: an ATS job ID either matches or it does not, so the number reflects
// how much the *rule* can be trusted, not how similar two strings were.
const (
	confidenceExactURL    = 1.00
	confidenceATSID       = 1.00
	confidenceFingerprint = 0.95
	// A shared description is strong but not conclusive: two genuinely
	// different roles at one company sometimes reuse the same boilerplate.
	confidenceContentHash = 0.90
)

// matchByCanonicalURL is tier 1: the same URL, once tracking noise is removed.
func (e *Engine) matchByCanonicalURL(ctx context.Context, tenantID domain.ID, normalized []Normalized, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	pending := map[string][]int{}
	for i, n := range normalized {
		if resolved[i] || n.CanonicalURL == "" {
			continue
		}
		pending[n.CanonicalURL] = append(pending[n.CanonicalURL], i)
	}
	if len(pending) == 0 {
		return nil
	}

	refs, err := e.postings.FindByCanonicalURLs(ctx, tenantID, keysOf(pending))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		for _, i := range pending[ref.CanonicalURL] {
			e.record(results, resolved, i, ref, domain.TierExactURL, confidenceExactURL,
				"same canonical url", policy)
		}
	}
	return nil
}

// matchByExternalID is tier 2: the same job on the same applicant tracking
// system, however it was reached.
func (e *Engine) matchByExternalID(ctx context.Context, tenantID domain.ID, normalized []Normalized, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	pending := map[string][]int{}
	keys := make([]domain.SourceKey, 0, len(normalized))
	for i, n := range normalized {
		if resolved[i] {
			continue
		}
		key, ok := n.SourceKey()
		if !ok {
			continue
		}
		if _, seen := pending[key.String()]; !seen {
			keys = append(keys, key)
		}
		pending[key.String()] = append(pending[key.String()], i)
	}
	if len(keys) == 0 {
		return nil
	}

	refs, err := e.postings.FindByExternalIDs(ctx, tenantID, keys)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		key := domain.SourceKey{Source: ref.Source, ExternalID: ref.ExternalID}
		for _, i := range pending[key.String()] {
			e.record(results, resolved, i, ref, domain.TierATSID, confidenceATSID,
				fmt.Sprintf("same %s job id %s", ref.Source, ref.ExternalID), policy)
		}
	}
	return nil
}

// matchByFingerprint is tier 3: the same role, company, and place, after
// normalisation. This is what catches a posting seen on two different boards.
func (e *Engine) matchByFingerprint(ctx context.Context, tenantID domain.ID, normalized []Normalized, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	pending := map[string][]int{}
	for i, n := range normalized {
		// A fingerprint of empty strings would match every other empty
		// posting, so candidates without identifying text skip this tier.
		if resolved[i] || n.Company == "" || n.Title == "" {
			continue
		}
		// A multi-site candidate ("Manhattan/SF/Philadelphia") is queried
		// under every site it could be stored as, not just its primary one:
		// a posting recorded from a single-site sighting of any of them is
		// still the same job.
		for _, fp := range n.FingerprintCandidates {
			pending[fp] = append(pending[fp], i)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	refs, err := e.postings.FindByFingerprints(ctx, tenantID, keysOf(pending))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		for _, i := range pending[ref.Fingerprint] {
			n := normalized[i]
			e.record(results, resolved, i, ref, domain.TierFingerprint, confidenceFingerprint,
				fmt.Sprintf("same normalised role %q at %q in %q", n.Title, n.Company, n.LocationBucket), policy)
		}
	}
	return nil
}

// matchByDescriptionHash is tier 4: identical posting text at the same
// employer. It catches the same job cross-posted under a title the earlier
// tiers could not reconcile.
//
// Scoped to one company, like the fuzzy tier: two different employers
// reposting the same agency or template boilerplate are not the same job, and
// content hashing alone cannot tell the difference.
func (e *Engine) matchByDescriptionHash(ctx context.Context, tenantID domain.ID, normalized []Normalized, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	pending := map[string][]int{}
	companyNames := map[string][]int{}
	for i, n := range normalized {
		if resolved[i] || n.DescriptionHash == "" || n.Company == "" {
			continue
		}
		pending[n.DescriptionHash] = append(pending[n.DescriptionHash], i)
		companyNames[n.Company] = append(companyNames[n.Company], i)
	}
	if len(pending) == 0 {
		return nil
	}

	companies, err := e.companies.FindByNormalizedNames(ctx, tenantID, keysOf(companyNames))
	if err != nil {
		return err
	}
	companyIDs := make([]domain.ID, 0, len(companies))
	for _, c := range companies {
		companyIDs = append(companyIDs, c.ID)
	}
	if len(companyIDs) == 0 {
		// None of the candidates' employers have any postings on record.
		return nil
	}

	refs, err := e.postings.FindByDescriptionHashes(ctx, tenantID, companyIDs, keysOf(pending))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		for _, i := range pending[ref.DescriptionHash] {
			n := normalized[i]
			company, known := companies[n.Company]
			if !known || company.ID != ref.CompanyID {
				continue
			}
			e.record(results, resolved, i, ref, domain.TierContentHash, confidenceContentHash,
				"identical posting description", policy)
		}
	}
	return nil
}

// matchByFuzzyTitle is tier 5, the only tier that infers rather than compares
// identifiers.
//
// It is scoped to a single company throughout: a title on its own is far too
// weak a signal, and comparing across employers would produce constant false
// positives ("Senior Software Engineer" exists everywhere).
func (e *Engine) matchByFuzzyTitle(ctx context.Context, tenantID domain.ID, normalized []Normalized, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	companyNames := map[string][]int{}
	for i, n := range normalized {
		if resolved[i] || n.Company == "" || n.Title == "" {
			continue
		}
		companyNames[n.Company] = append(companyNames[n.Company], i)
	}
	if len(companyNames) == 0 {
		return nil
	}

	companies, err := e.companies.FindByNormalizedNames(ctx, tenantID, keysOf(companyNames))
	if err != nil {
		return err
	}

	for name, indices := range companyNames {
		company, known := companies[name]
		if !known {
			// An employer with no postings on record cannot have a duplicate.
			continue
		}
		for _, i := range indices {
			if err := e.fuzzyMatchOne(ctx, tenantID, company.ID, normalized[i], i, results, resolved, policy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) fuzzyMatchOne(ctx context.Context, tenantID, companyID domain.ID, n Normalized, i int, results []domain.FilterResult, resolved []bool, policy domain.ResurfacePolicy) error {
	shortlist, err := e.postings.FindFuzzyCandidates(ctx, tenantID, companyID, n.Title, e.cfg.FuzzyCandidateLimit)
	if err != nil {
		return err
	}
	if len(shortlist) == 0 {
		return nil
	}

	best := domain.PostingRef{}
	bestScore := 0.0
	for _, ref := range shortlist {
		// A role in one city and an onsite role in another are not the same
		// job even when their titles are identical.
		if !compatibleLocations(n.LocationBucketCandidates, ref.LocationBucket) {
			continue
		}
		score := TitleSimilarity(n.Title, ref.NormalizedTitle)
		if score > bestScore {
			best, bestScore = ref, score
		}
	}
	if bestScore < e.cfg.ReviewThreshold {
		return nil
	}

	differing := DifferingTokens(n.Title, best.NormalizedTitle)
	sort.Strings(differing)

	if bestScore >= e.cfg.MatchThreshold {
		reason := fmt.Sprintf("title %.0f%% similar to an existing posting at the same company", bestScore*100)
		if len(differing) > 0 {
			reason += fmt.Sprintf(" (differing words: %v)", differing)
		}
		e.record(results, resolved, i, best, domain.TierFuzzy, bestScore, reason, policy)
		return nil
	}

	// The ambiguous band. Reporting needs_review rather than guessing is the
	// point: a wrong duplicate silently costs an opportunity, and a wrong new
	// costs a duplicate application. Neither is the engine's call to make.
	results[i] = domain.FilterResult{
		Index:             i,
		Verdict:           domain.VerdictNeedsReview,
		Tier:              domain.TierFuzzy,
		Confidence:        bestScore,
		PostingID:         &best.PostingID,
		ApplicationID:     best.ApplicationID,
		ApplicationStatus: best.ApplicationStatus,
		FirstSeenAt:       &best.FirstSeenAt,
		LastSeenAt:        &best.LastSeenAt,
		DaysSince:         daysSince(best.LastSeenAt, e.now()),
		Reason: fmt.Sprintf(
			"title %.0f%% similar to an existing posting at the same company (differing words: %v); too close to call",
			bestScore*100, differing),
	}
	resolved[i] = true
	return nil
}

// compatibleLocations reports whether a candidate's location could describe
// the same job as an existing posting's stored bucket.
//
// An unknown bucket on either side is treated as compatible with anything,
// because a missing location is an absence of evidence rather than a
// difference. "remote" is treated as compatible with any specific place too:
// boards disagree about whether a distributed role "is" remote or "is" the
// city its team sits in, and in practice that disagreement is about the same
// job, not two different ones.
func compatibleLocations(candidateBuckets []string, existing string) bool {
	if existing == "" || len(candidateBuckets) == 0 {
		return true
	}
	if existing == "remote" {
		return true
	}
	for _, b := range candidateBuckets {
		if b == "remote" || b == existing {
			return true
		}
	}
	return false
}

// record applies the resurfacing policy to a match and writes the result.
//
// This is the one place a match becomes a verdict, so the policy cannot be
// bypassed by a tier that forgets to consult it.
func (e *Engine) record(results []domain.FilterResult, resolved []bool, i int, ref domain.PostingRef, tier domain.MatchTier, confidence float64, reason string, policy domain.ResurfacePolicy) {
	if resolved[i] {
		return
	}

	now := e.now()
	status := ref.EffectiveStatus()
	eligible, policyReason := policy.Eligible(status, ref.LastSeenAt, now)

	verdict := domain.VerdictDuplicate
	if eligible {
		verdict = domain.VerdictEligibleAgain
	}

	postingID := ref.PostingID
	firstSeen := ref.FirstSeenAt
	lastSeen := ref.LastSeenAt

	results[i] = domain.FilterResult{
		Index:             i,
		Verdict:           verdict,
		Tier:              tier,
		Confidence:        confidence,
		PostingID:         &postingID,
		ApplicationID:     ref.ApplicationID,
		ApplicationStatus: ref.ApplicationStatus,
		FirstSeenAt:       &firstSeen,
		LastSeenAt:        &lastSeen,
		DaysSince:         daysSince(lastSeen, now),
		Reason:            reason + "; " + policyReason,
	}
	resolved[i] = true
}

func daysSince(then, now time.Time) *int {
	if then.IsZero() {
		return nil
	}
	days := int(math.Floor(now.Sub(then).Hours() / 24))
	if days < 0 {
		days = 0
	}
	return &days
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic query parameters make failures reproducible
	return keys
}
