package postgres_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/testsupport"
)

// newTestStore returns a store and a tenant scoped to the calling test.
//
// Tests isolate from each other by working under their own tenant rather than
// by resetting the database. That is both faster and a genuine exercise of the
// tenant scoping every query relies on.
func newTestStore(t *testing.T) (store.Store, domain.ID) {
	t.Helper()
	return testsupport.Store(t)
}

// seedPosting inserts a company and a posting, returning the posting.
func seedPosting(t *testing.T, s store.Store, tenantID domain.ID, p domain.Posting) domain.Posting {
	t.Helper()
	ctx := t.Context()

	company, err := s.Companies().Upsert(ctx, domain.Company{
		TenantID:       tenantID,
		Name:           "Acme, Inc.",
		NormalizedName: "acme",
	})
	require.NoError(t, err)

	p.TenantID = tenantID
	p.CompanyID = company.ID
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
	}
	inserted, err := s.Postings().Insert(ctx, p)
	require.NoError(t, err)
	return inserted
}
