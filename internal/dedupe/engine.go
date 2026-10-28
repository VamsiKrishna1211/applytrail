package dedupe

import (
	"context"
	"fmt"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Lookup is the read surface the engine needs from storage.
//
// It is declared here, at the point of use, rather than in the store package,
// so the engine can be exercised against an in-memory fake without pulling in
// a database. store.PostingStore satisfies it structurally.
type Lookup interface {
	FindByCanonicalURLs(ctx context.Context, tenantID domain.ID, urls []string) ([]domain.PostingRef, error)
	FindByExternalIDs(ctx context.Context, tenantID domain.ID, keys []domain.SourceKey) ([]domain.PostingRef, error)
	FindByFingerprints(ctx context.Context, tenantID domain.ID, fingerprints []string) ([]domain.PostingRef, error)
	FindByDescriptionHashes(ctx context.Context, tenantID domain.ID, companyIDs []domain.ID, hashes []string) ([]domain.PostingRef, error)
	FindFuzzyCandidates(ctx context.Context, tenantID, companyID domain.ID, normalizedTitle string, limit int) ([]domain.PostingRef, error)
}

// CompanyLookup resolves employers by normalized name.
type CompanyLookup interface {
	FindByNormalizedNames(ctx context.Context, tenantID domain.ID, normalizedNames []string) (map[string]domain.Company, error)
}

// Matcher is the port a match strategy implements. Filter takes a whole batch
// because the tiered strategy resolves each tier with a single query, and
// because candidates within one batch can duplicate each other.
type Matcher interface {
	Filter(ctx context.Context, tenantID domain.ID, candidates []domain.Candidate, policy domain.ResurfacePolicy) ([]domain.FilterResult, error)
}

// Config tunes the fuzzy tier.
type Config struct {
	// MatchThreshold is the similarity at or above which a fuzzy match is
	// reported as a duplicate.
	MatchThreshold float64
	// ReviewThreshold is the similarity at or above which a fuzzy match is
	// reported for human or model review. Below it, nothing is reported.
	ReviewThreshold float64
	// FuzzyCandidateLimit caps how many postings the trigram shortlist returns
	// per candidate.
	FuzzyCandidateLimit int
}

// DefaultConfig returns the tuning the service uses when nothing overrides it.
func DefaultConfig() Config {
	return Config{
		MatchThreshold:      0.87,
		ReviewThreshold:     0.75,
		FuzzyCandidateLimit: 20,
	}
}

// Engine is the layered match strategy.
type Engine struct {
	postings  Lookup
	companies CompanyLookup
	cfg       Config
	now       func() time.Time
}

var _ Matcher = (*Engine)(nil)

// NewEngine builds an Engine. A zero-valued field in cfg falls back to the
// default, so a caller can override one threshold without restating the rest.
func NewEngine(postings Lookup, companies CompanyLookup, cfg Config) *Engine {
	defaults := DefaultConfig()
	if cfg.MatchThreshold <= 0 {
		cfg.MatchThreshold = defaults.MatchThreshold
	}
	if cfg.ReviewThreshold <= 0 {
		cfg.ReviewThreshold = defaults.ReviewThreshold
	}
	if cfg.FuzzyCandidateLimit <= 0 {
		cfg.FuzzyCandidateLimit = defaults.FuzzyCandidateLimit
	}
	return &Engine{postings: postings, companies: companies, cfg: cfg, now: time.Now}
}

// WithClock overrides the time source, for tests that need a fixed now.
func (e *Engine) WithClock(now func() time.Time) *Engine {
	e.now = now
	return e
}

// Normalized is a candidate reduced to its comparable keys. Computing these
// once per candidate keeps every tier cheap and keeps ingest consistent with
// dedupe: both derive a posting's stored columns from this same struct.
type Normalized struct {
	Candidate domain.Candidate

	CanonicalURL string
	Identity     Identity
	Company      string
	Title        string
	// LocationBucket is the single value stored on a posting row: the
	// primary site a multi-site location names, or the sole bucket otherwise.
	LocationBucket string
	// LocationBucketCandidates is every plausible bucket the location could
	// mean, used only for matching. A multi-site candidate ("Manhattan/SF/
	// Philadelphia") can therefore be recognised against a posting stored
	// under any one of its sites, even though only the primary site is ever
	// persisted. Always a superset of {LocationBucket}.
	LocationBucketCandidates []string
	Fingerprint              string
	// FingerprintCandidates mirrors LocationBucketCandidates: one fingerprint
	// per plausible location bucket, so tier 3 can match a multi-site
	// candidate against a posting stored under any one of its sites.
	FingerprintCandidates []string
	DescriptionHash       string
}

// Normalize reduces a candidate to its comparable keys.
//
// When the caller supplied no company name, the employer slug from the ATS URL
// stands in: a Greenhouse board URL names the company even when the scraped
// text did not.
func Normalize(c domain.Candidate) Normalized {
	identity := ExtractIdentity(c.URL)

	company := NormalizeCompany(c.Company)
	if company == "" {
		company = NormalizeCompany(identity.CompanySlug)
	}

	title := NormalizeTitle(c.Title)
	buckets := BucketLocationCandidates(c.Location)
	bucket := ""
	if len(buckets) > 0 {
		bucket = buckets[0]
	}

	fingerprints := make([]string, 0, len(buckets)+1)
	seen := make(map[string]bool, len(buckets)+1)
	for _, b := range buckets {
		fp := Fingerprint(company, title, b)
		if !seen[fp] {
			seen[fp] = true
			fingerprints = append(fingerprints, fp)
		}
	}
	if len(fingerprints) == 0 {
		// No location signal at all: fall back to the single unlocated
		// fingerprint so this candidate is still matchable.
		fingerprints = append(fingerprints, Fingerprint(company, title, ""))
	}

	return Normalized{
		Candidate:                c,
		CanonicalURL:             CanonicalizeURL(c.URL),
		Identity:                 identity,
		Company:                  company,
		Title:                    title,
		LocationBucket:           bucket,
		LocationBucketCandidates: buckets,
		Fingerprint:              Fingerprint(company, title, bucket),
		FingerprintCandidates:    fingerprints,
		DescriptionHash:          DescriptionHash(c.Description),
	}
}

// Source returns the posting's origin: what the URL revealed, or what the
// caller declared when the URL was not a recognised board.
func (n Normalized) Source() domain.Source {
	if n.Identity.Source != domain.SourceUnknown {
		return n.Identity.Source
	}
	return n.Candidate.Source
}

// ExternalID is the value stored in the posting's external_id column: the ATS
// job ID qualified by employer slug. Ingest and dedupe must agree on this, so
// both read it from here rather than deriving it separately.
func (n Normalized) ExternalID() string { return n.Identity.Key() }

// SourceKey returns the ATS identity, and whether there is one.
func (n Normalized) SourceKey() (domain.SourceKey, bool) {
	if !n.Identity.Found() {
		return domain.SourceKey{}, false
	}
	return domain.SourceKey{Source: n.Identity.Source, ExternalID: n.Identity.Key()}, true
}

// Filter resolves every candidate against what the tenant has already seen.
//
// Tiers run cheapest-first over the whole batch, and each tier only queries
// for the candidates still unresolved. A batch of 200 candidates therefore
// costs at most five batched queries plus one shortlist query per candidate
// that reaches the fuzzy tier.
func (e *Engine) Filter(ctx context.Context, tenantID domain.ID, candidates []domain.Candidate, policy domain.ResurfacePolicy) ([]domain.FilterResult, error) {
	results := make([]domain.FilterResult, len(candidates))
	if len(candidates) == 0 {
		return results, nil
	}

	normalized := make([]Normalized, len(candidates))
	for i, c := range candidates {
		normalized[i] = Normalize(c)
		results[i] = domain.FilterResult{Index: i, Verdict: domain.VerdictNew, Tier: domain.TierNone}
	}

	// resolved marks candidates a tier has already answered, so later tiers
	// skip them entirely.
	resolved := make([]bool, len(candidates))

	// Candidates can duplicate each other inside a single request; catching
	// that here stops one batch from inserting the same job twice.
	e.markIntraBatchDuplicates(normalized, results, resolved)

	tiers := []struct {
		name string
		run  func(context.Context, domain.ID, []Normalized, []domain.FilterResult, []bool, domain.ResurfacePolicy) error
	}{
		{"exact_url", e.matchByCanonicalURL},
		{"ats_id", e.matchByExternalID},
		{"fingerprint", e.matchByFingerprint},
		{"content_hash", e.matchByDescriptionHash},
		{"fuzzy", e.matchByFuzzyTitle},
	}
	for _, tier := range tiers {
		if err := tier.run(ctx, tenantID, normalized, results, resolved, policy); err != nil {
			return nil, fmt.Errorf("dedupe tier %s: %w", tier.name, err)
		}
	}

	for i := range results {
		if results[i].Verdict == domain.VerdictNew && results[i].Reason == "" {
			results[i].Reason = "no match on url, ats id, fingerprint, description, or title similarity"
		}
	}
	return results, nil
}

// markIntraBatchDuplicates collapses candidates that duplicate each other
// within the request, keeping the first occurrence and pointing the rest at it.
func (e *Engine) markIntraBatchDuplicates(normalized []Normalized, results []domain.FilterResult, resolved []bool) {
	firstByURL := map[string]int{}
	firstByATS := map[string]int{}
	firstByFingerprint := map[string]int{}

	for i, n := range normalized {
		claim := func(key string, index map[string]int, tier domain.MatchTier) bool {
			if key == "" {
				return false
			}
			if first, seen := index[key]; seen {
				results[i] = domain.FilterResult{
					Index:      i,
					Verdict:    domain.VerdictDuplicate,
					Tier:       tier,
					Confidence: 1,
					Reason:     fmt.Sprintf("duplicate of candidate %d in this same request", first),
				}
				resolved[i] = true
				return true
			}
			index[key] = i
			return false
		}

		if claim(n.CanonicalURL, firstByURL, domain.TierExactURL) {
			continue
		}
		if key, ok := n.SourceKey(); ok && claim(key.String(), firstByATS, domain.TierATSID) {
			continue
		}
		// A fingerprint over empty company and title is meaningless, so only
		// index candidates that carry real identifying text.
		if n.Company != "" && n.Title != "" {
			claim(n.Fingerprint, firstByFingerprint, domain.TierFingerprint)
		}
	}
}
