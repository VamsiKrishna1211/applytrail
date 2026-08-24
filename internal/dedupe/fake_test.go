package dedupe_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// fakeStore is an in-memory stand-in for the posting and company lookups.
//
// The engine's own logic is what these tests exercise, so running them against
// a map rather than Postgres keeps them fast and makes every fixture visible
// at the point of use. The SQL behaviour these fakes stand in for is covered
// separately by the store's integration tests.
type fakeStore struct {
	tenantID  domain.ID
	postings  []fakePosting
	companies map[string]domain.Company
}

type fakePosting struct {
	ref       domain.PostingRef
	companyID domain.ID
}

func newFakeStore(tenantID domain.ID) *fakeStore {
	return &fakeStore{tenantID: tenantID, companies: map[string]domain.Company{}}
}

// seed records an existing posting, deriving its match keys through the same
// normalizers the engine uses so the fixture cannot drift from production.
func (f *fakeStore) seed(t *testing.T, companyName string, c domain.Candidate, status domain.Status, lastSeen time.Time) domain.ID {
	t.Helper()

	normalizedCompany := dedupe.NormalizeCompany(companyName)
	company, ok := f.companies[normalizedCompany]
	if !ok {
		company = domain.Company{
			ID:             domain.NewID(),
			TenantID:       f.tenantID,
			Name:           companyName,
			NormalizedName: normalizedCompany,
		}
		f.companies[normalizedCompany] = company
	}

	c.Company = companyName
	n := dedupe.Normalize(c)

	postingID := domain.NewID()
	applicationID := domain.NewID()
	f.postings = append(f.postings, fakePosting{
		companyID: company.ID,
		ref: domain.PostingRef{
			PostingID:         postingID,
			CanonicalURL:      n.CanonicalURL,
			Source:            n.Source(),
			ExternalID:        n.ExternalID(),
			Fingerprint:       n.Fingerprint,
			DescriptionHash:   n.DescriptionHash,
			NormalizedTitle:   n.Title,
			LocationBucket:    n.LocationBucket,
			FirstSeenAt:       lastSeen,
			LastSeenAt:        lastSeen,
			ApplicationID:     &applicationID,
			ApplicationStatus: &status,
		},
	})
	return postingID
}

func (f *fakeStore) FindByCanonicalURLs(_ context.Context, tenantID domain.ID, urls []string) ([]domain.PostingRef, error) {
	return f.filter(tenantID, func(p fakePosting) bool {
		return p.ref.CanonicalURL != "" && contains(urls, p.ref.CanonicalURL)
	}), nil
}

func (f *fakeStore) FindByExternalIDs(_ context.Context, tenantID domain.ID, keys []domain.SourceKey) ([]domain.PostingRef, error) {
	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		wanted[k.String()] = true
	}
	return f.filter(tenantID, func(p fakePosting) bool {
		if p.ref.ExternalID == "" {
			return false
		}
		return wanted[domain.SourceKey{Source: p.ref.Source, ExternalID: p.ref.ExternalID}.String()]
	}), nil
}

func (f *fakeStore) FindByFingerprints(_ context.Context, tenantID domain.ID, fingerprints []string) ([]domain.PostingRef, error) {
	return f.filter(tenantID, func(p fakePosting) bool {
		return contains(fingerprints, p.ref.Fingerprint)
	}), nil
}

func (f *fakeStore) FindByDescriptionHashes(_ context.Context, tenantID domain.ID, hashes []string) ([]domain.PostingRef, error) {
	return f.filter(tenantID, func(p fakePosting) bool {
		return p.ref.DescriptionHash != "" && contains(hashes, p.ref.DescriptionHash)
	}), nil
}

// FindFuzzyCandidates mimics the pg_trgm shortlist: same company, and enough
// shared text to be worth scoring. The exact scoring is the engine's job.
func (f *fakeStore) FindFuzzyCandidates(_ context.Context, tenantID, companyID domain.ID, normalizedTitle string, limit int) ([]domain.PostingRef, error) {
	var out []domain.PostingRef
	for _, p := range f.postings {
		if p.ref.PostingID == domain.ID(uuidNil) || p.companyID != companyID {
			continue
		}
		if !sharesAnyToken(normalizedTitle, p.ref.NormalizedTitle) {
			continue
		}
		out = append(out, p.ref)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeStore) FindByNormalizedNames(_ context.Context, _ domain.ID, names []string) (map[string]domain.Company, error) {
	found := map[string]domain.Company{}
	for _, name := range names {
		if c, ok := f.companies[name]; ok {
			found[name] = c
		}
	}
	return found, nil
}

func (f *fakeStore) filter(tenantID domain.ID, keep func(fakePosting) bool) []domain.PostingRef {
	if tenantID != f.tenantID {
		return nil
	}
	var out []domain.PostingRef
	for _, p := range f.postings {
		if keep(p) {
			out = append(out, p.ref)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func sharesAnyToken(a, b string) bool {
	for _, token := range strings.Fields(a) {
		if len(token) > 3 && strings.Contains(b, token) {
			return true
		}
	}
	return false
}

var uuidNil domain.ID
