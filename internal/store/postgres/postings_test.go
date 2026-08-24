package postgres_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

func TestPostingStore_FindByCanonicalURLs(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		Source:          domain.SourceGreenhouse,
		ExternalID:      "4012345",
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/4012345",
		Title:           "Senior Software Engineer II",
		NormalizedTitle: "senior software engineer",
		LocationBucket:  "remote",
		Fingerprint:     "fp-senior-swe-remote",
	})

	refs, err := s.Postings().FindByCanonicalURLs(ctx, tenantID, []string{
		posting.CanonicalURL,
		"https://example.com/never-seen",
	})
	require.NoError(t, err)
	require.Len(t, refs, 1, "only the known URL should match")
	assert.Equal(t, posting.ID, refs[0].PostingID)
	assert.Nil(t, refs[0].ApplicationStatus, "a posting with no application has no status")

	t.Run("empty input issues no query", func(t *testing.T) {
		refs, err := s.Postings().FindByCanonicalURLs(ctx, tenantID, nil)
		require.NoError(t, err)
		assert.Empty(t, refs)
	})
}

func TestPostingStore_FindByExternalIDs(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		Source:          domain.SourceGreenhouse,
		ExternalID:      "4012345",
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/4012345",
		Title:           "Senior Software Engineer II",
		NormalizedTitle: "senior software engineer",
		Fingerprint:     "fp-ats",
	})

	refs, err := s.Postings().FindByExternalIDs(ctx, tenantID, []domain.SourceKey{
		{Source: domain.SourceGreenhouse, ExternalID: "4012345"},
	})
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, posting.ID, refs[0].PostingID)

	t.Run("same id on a different board is a different job", func(t *testing.T) {
		refs, err := s.Postings().FindByExternalIDs(ctx, tenantID, []domain.SourceKey{
			{Source: domain.SourceLever, ExternalID: "4012345"},
		})
		require.NoError(t, err)
		assert.Empty(t, refs, "numeric ids collide across boards; source must disambiguate")
	})
}

func TestPostingStore_FindFuzzyCandidates(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		Source:          domain.SourceGreenhouse,
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/1",
		Title:           "Senior Software Engineer",
		NormalizedTitle: "senior software engineer",
		LocationBucket:  "remote",
		Fingerprint:     "fp-fuzzy",
	})

	refs, err := s.Postings().FindFuzzyCandidates(ctx, tenantID, posting.CompanyID, "senior software engineer backend", 10)
	require.NoError(t, err)
	require.NotEmpty(t, refs, "a near-identical title should be shortlisted")
	assert.Equal(t, posting.ID, refs[0].PostingID)
	assert.Greater(t, refs[0].Similarity, 0.3, "trigram score should be reported")

	t.Run("unrelated title is not shortlisted", func(t *testing.T) {
		refs, err := s.Postings().FindFuzzyCandidates(ctx, tenantID, posting.CompanyID, "warehouse forklift operator", 10)
		require.NoError(t, err)
		assert.Empty(t, refs)
	})
}

func TestPostingStore_TouchSeenOnlyMovesForward(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	now := time.Now().UTC().Truncate(time.Millisecond)
	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/2",
		Title:           "Staff Engineer",
		NormalizedTitle: "staff engineer",
		Fingerprint:     "fp-touch",
		FirstSeenAt:     now,
	})

	later := now.Add(24 * time.Hour)
	updated, err := s.Postings().TouchSeen(ctx, tenantID, posting.ID, later)
	require.NoError(t, err)
	assert.WithinDuration(t, later, updated.LastSeenAt, time.Second)

	// An ingest arriving out of order must not rewind the clock, otherwise a
	// stale batch would make a fresh posting look abandoned.
	earlier := now.Add(-24 * time.Hour)
	rewound, err := s.Postings().TouchSeen(ctx, tenantID, posting.ID, earlier)
	require.NoError(t, err)
	assert.WithinDuration(t, later, rewound.LastSeenAt, time.Second, "last_seen_at must never move backwards")
}

func TestPostingStore_TenantIsolation(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/3",
		Title:           "Platform Engineer",
		NormalizedTitle: "platform engineer",
		Fingerprint:     "fp-isolation",
	})

	other, err := s.Tenants().Create(ctx, "other", "other-"+domain.NewID().String())
	require.NoError(t, err)

	refs, err := s.Postings().FindByCanonicalURLs(ctx, other.ID, []string{posting.CanonicalURL})
	require.NoError(t, err)
	assert.Empty(t, refs, "one tenant must never see another tenant's postings")

	_, err = s.Postings().Get(ctx, other.ID, posting.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPostingStore_DuplicateFingerprintConflicts(t *testing.T) {
	s, tenantID := newTestStore(t)
	ctx := t.Context()

	posting := seedPosting(t, s, tenantID, domain.Posting{
		CanonicalURL:    "https://boards.greenhouse.io/acme/jobs/4",
		Title:           "Backend Engineer",
		NormalizedTitle: "backend engineer",
		Fingerprint:     "fp-unique",
	})

	_, err := s.Postings().Insert(ctx, domain.Posting{
		TenantID:        tenantID,
		CompanyID:       posting.CompanyID,
		CanonicalURL:    "https://different.example/job/9",
		Title:           "Backend Engineer",
		NormalizedTitle: "backend engineer",
		Fingerprint:     "fp-unique",
	})
	require.ErrorIs(t, err, domain.ErrConflict,
		"the fingerprint uniqueness constraint is what stops a duplicate slipping in under a new URL")
}
