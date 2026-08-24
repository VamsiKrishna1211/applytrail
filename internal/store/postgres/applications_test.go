package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

func TestApplicationStore_InsertIsIdempotent(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/10",
		Title:           "Senior Engineer",
		NormalizedTitle: "senior engineer",
		Fingerprint:     "fp-app-idempotent",
	})

	first, err := s.Applications().Insert(ctx, domain.Application{
		TenantID:  tenantID,
		PostingID: posting.ID,
		Status:    domain.StatusDiscovered,
	})
	require.NoError(t, err)

	// A retried ingest must resolve to the same application rather than
	// failing on the uniqueness constraint.
	second, err := s.Applications().Insert(ctx, domain.Application{
		TenantID:  tenantID,
		PostingID: posting.ID,
		Status:    domain.StatusDiscovered,
	})
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "re-inserting for the same posting returns the existing row")
}

func TestApplicationStore_TransitionIsCompareAndSwap(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/11",
		Title:           "Senior Engineer",
		NormalizedTitle: "senior engineer",
		Fingerprint:     "fp-app-cas",
	})
	app, err := s.Applications().Insert(ctx, domain.Application{
		TenantID:  tenantID,
		PostingID: posting.ID,
		Status:    domain.StatusDiscovered,
	})
	require.NoError(t, err)

	appliedAt := time.Now().UTC()
	moved, err := s.Applications().ApplyTransition(ctx, tenantID, app.ID, store.StatusTransition{
		From:      domain.StatusDiscovered,
		To:        domain.StatusApplied,
		AppliedAt: &appliedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApplied, moved.Status)
	require.NotNil(t, moved.AppliedAt)

	// Replaying the same transition must fail: the row no longer reads
	// "discovered", so a stale writer loses instead of clobbering.
	_, err = s.Applications().ApplyTransition(ctx, tenantID, app.ID, store.StatusTransition{
		From: domain.StatusDiscovered,
		To:   domain.StatusRejected,
	})
	require.ErrorIs(t, err, domain.ErrConflict)
	assert.Contains(t, err.Error(), string(domain.StatusApplied), "the error should name the status it actually found")
}

func TestApplicationStore_FollowupQueue(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	now := time.Now().UTC()
	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/12",
		Title:           "Senior Engineer",
		NormalizedTitle: "senior engineer",
		Fingerprint:     "fp-followup",
	})

	due := now.Add(-24 * time.Hour)
	app, err := s.Applications().Insert(ctx, domain.Application{
		TenantID:     tenantID,
		PostingID:    posting.ID,
		Status:       domain.StatusApplied,
		AppliedAt:    &now,
		NextActionAt: &due,
	})
	require.NoError(t, err)

	overdue, err := s.Applications().ListFollowupsDue(ctx, tenantID, now, 10)
	require.NoError(t, err)
	require.Len(t, overdue, 1)
	assert.Equal(t, app.ID, overdue[0].Application.ID)
	assert.Equal(t, "Senior Engineer", overdue[0].PostingTitle, "the queue joins enough detail to act on")

	t.Run("terminal applications drop out of the queue", func(t *testing.T) {
		_, err := s.Applications().ApplyTransition(ctx, tenantID, app.ID, store.StatusTransition{
			From: domain.StatusApplied,
			To:   domain.StatusRejected,
		})
		require.NoError(t, err)

		overdue, err := s.Applications().ListFollowupsDue(ctx, tenantID, now, 10)
		require.NoError(t, err)
		assert.Empty(t, overdue, "there is nothing to chase once the answer is no")
	})
}

func TestApplicationStore_AppliedAtIsEnforcedBySchema(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/13",
		Title:           "Senior Engineer",
		NormalizedTitle: "senior engineer",
		Fingerprint:     "fp-applied-at",
	})

	// The check constraint exists so a manual SQL fix cannot produce an
	// applied application with no application date, which would silently
	// corrupt the funnel report.
	_, err := s.Applications().Insert(ctx, domain.Application{
		TenantID:  tenantID,
		PostingID: posting.ID,
		Status:    domain.StatusApplied,
	})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
}

func TestStore_TxRollsBackOnError(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	sentinel := assert.AnError
	err := s.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		_, err := tx.Companies().Upsert(ctx, domain.Company{
			TenantID:       tenantID,
			Name:           "Rollback Co",
			NormalizedName: "rollback co",
		})
		require.NoError(t, err)
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	_, err = s.Companies().GetByNormalizedName(ctx, tenantID, "rollback co")
	require.ErrorIs(t, err, domain.ErrNotFound, "the failed transaction must leave nothing behind")
}
