package dedupe_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// now is the fixed clock every engine test runs against, so "90 days ago"
// means the same thing on every machine and in every season.
var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func newEngine(t *testing.T) (*dedupe.Engine, *fakeStore, domain.ID) {
	t.Helper()
	tenantID := domain.NewID()
	store := newFakeStore(tenantID)
	engine := dedupe.NewEngine(store, store, dedupe.DefaultConfig()).
		WithClock(func() time.Time { return now })
	return engine, store, tenantID
}

func filterOne(t *testing.T, e *dedupe.Engine, tenantID domain.ID, c domain.Candidate) domain.FilterResult {
	t.Helper()
	results, err := e.Filter(t.Context(), tenantID, []domain.Candidate{c}, domain.DefaultResurfacePolicy())
	require.NoError(t, err)
	require.Len(t, results, 1)
	return results[0]
}

func TestEngine_UnseenCandidateIsNew(t *testing.T) {
	engine, _, tenantID := newEngine(t)

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:     "https://boards.greenhouse.io/acme/jobs/4012345",
		Title:   "Senior Software Engineer II",
		Company: "Acme, Inc.",
	})

	assert.Equal(t, domain.VerdictNew, got.Verdict)
	assert.Equal(t, domain.TierNone, got.Tier)
	assert.Nil(t, got.PostingID)
}

func TestEngine_Tier1_ExactURLSurvivesTrackingParameters(t *testing.T) {
	engine, store, tenantID := newEngine(t)
	postingID := store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:   "https://boards.greenhouse.io/acme/jobs/4012345",
		Title: "Senior Software Engineer II",
	}, domain.StatusApplied, now.AddDate(0, 0, -14))

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:     "https://boards.greenhouse.io/acme/jobs/4012345?utm_source=newsletter",
		Title:   "Senior Software Engineer II",
		Company: "Acme, Inc.",
	})

	assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	assert.Equal(t, domain.TierExactURL, got.Tier)
	assert.Equal(t, 1.0, got.Confidence)
	require.NotNil(t, got.PostingID)
	assert.Equal(t, postingID, *got.PostingID)
	require.NotNil(t, got.DaysSince)
	assert.Equal(t, 14, *got.DaysSince, "the caller gets the facts, not just the verdict")
}

func TestEngine_Tier2_SameATSJobReachedByAnotherRoute(t *testing.T) {
	engine, store, tenantID := newEngine(t)
	store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:   "https://boards.greenhouse.io/acme/jobs/4012345",
		Title: "Senior Software Engineer II",
	}, domain.StatusApplied, now.AddDate(0, 0, -3))

	// Same Greenhouse job, but on the newer host and with a different title
	// rendering, so tier 1 cannot see it.
	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:     "https://job-boards.greenhouse.io/acme/jobs/4012345",
		Title:   "Senior Engineer",
		Company: "Acme",
	})

	assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	assert.Equal(t, domain.TierATSID, got.Tier)
	assert.Contains(t, got.Reason, "4012345")
}

// The headline case: the same job listed on the company's Greenhouse board and
// on LinkedIn, with the title abbreviated differently. Neither the URL nor the
// ATS identity lines up, so only normalisation can catch it.
func TestEngine_Tier3_SameJobOnLinkedInAndTheCareersPage(t *testing.T) {
	engine, store, tenantID := newEngine(t)
	store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:      "https://boards.greenhouse.io/acme/jobs/4012345",
		Title:    "Senior Software Engineer II",
		Location: "Remote",
	}, domain.StatusApplied, now.AddDate(0, 0, -20))

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:      "https://www.linkedin.com/jobs/view/9988776655",
		Title:    "Sr. SWE II",
		Company:  "Acme",
		Location: "Remote - US",
	})

	assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	assert.Equal(t, domain.TierFingerprint, got.Tier)
	assert.Equal(t, domain.StatusApplied, *got.ApplicationStatus)
}

func TestEngine_Tier4_IdenticalDescriptionUnderADifferentTitle(t *testing.T) {
	engine, store, tenantID := newEngine(t)

	description := "We are hiring an engineer to design and operate distributed systems " +
		"serving millions of requests per second. You will own services end to end, " +
		"from design review through production operation, and mentor other engineers " +
		"on the platform team. Experience with Go, Postgres, and Kubernetes is required."

	store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:         "https://boards.greenhouse.io/acme/jobs/1",
		Title:       "Distributed Systems Engineer",
		Description: description,
	}, domain.StatusDiscovered, now.AddDate(0, 0, -2))

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:         "https://acme.com/careers/platform-role",
		Title:       "Platform Infrastructure Specialist",
		Company:     "Acme",
		Description: description,
	})

	assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	assert.Equal(t, domain.TierContentHash, got.Tier)
}

func TestEngine_Tier5_FuzzyTitle(t *testing.T) {
	t.Run("near-identical title is a duplicate", func(t *testing.T) {
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Acme, Inc.", domain.Candidate{
			URL:      "https://boards.greenhouse.io/acme/jobs/1",
			Title:    "Senior Software Engineer",
			Location: "Remote",
		}, domain.StatusApplied, now.AddDate(0, 0, -5))

		// Different location text, different board, and a title that only
		// differs by a level marker that normalisation removes.
		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://acme.com/careers/2",
			Title:    "Senior Software Engineer III",
			Company:  "Acme",
			Location: "Remote",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	})

	t.Run("an extra qualifier is too close to call", func(t *testing.T) {
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Acme, Inc.", domain.Candidate{
			URL:      "https://boards.greenhouse.io/acme/jobs/1",
			Title:    "Senior Software Engineer",
			Location: "Remote",
		}, domain.StatusApplied, now.AddDate(0, 0, -5))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://acme.com/careers/3",
			Title:    "Senior Software Engineer, Payments",
			Company:  "Acme",
			Location: "Remote",
		})

		assert.Equal(t, domain.VerdictNeedsReview, got.Verdict,
			"a team qualifier could be the same job or a sibling role; the engine must not guess")
		assert.Equal(t, domain.TierFuzzy, got.Tier)
		assert.Contains(t, got.Reason, "payments", "the reason names what differed")
		require.NotNil(t, got.PostingID, "the caller needs the candidate match to adjudicate")
	})

	t.Run("the same title at a different company is not a match", func(t *testing.T) {
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Acme, Inc.", domain.Candidate{
			URL:   "https://boards.greenhouse.io/acme/jobs/1",
			Title: "Senior Software Engineer",
		}, domain.StatusApplied, now.AddDate(0, 0, -5))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:     "https://boards.greenhouse.io/globex/jobs/1",
			Title:   "Senior Software Engineer",
			Company: "Globex",
		})
		assert.Equal(t, domain.VerdictNew, got.Verdict,
			"generic titles exist everywhere; fuzzy matching must stay scoped to one employer")
	})

	t.Run("different cities are different jobs", func(t *testing.T) {
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Acme, Inc.", domain.Candidate{
			URL:      "https://boards.greenhouse.io/acme/jobs/1",
			Title:    "Senior Software Engineer",
			Location: "Austin, TX",
		}, domain.StatusApplied, now.AddDate(0, 0, -5))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://acme.com/careers/4",
			Title:    "Senior Software Engineer",
			Company:  "Acme",
			Location: "Berlin, Germany",
		})
		assert.Equal(t, domain.VerdictNew, got.Verdict)
	})
}

func TestEngine_ResurfacingPolicy(t *testing.T) {
	tests := []struct {
		name        string
		status      domain.Status
		daysAgo     int
		wantVerdict domain.Verdict
		why         string
	}{
		{
			name: "applied never resurfaces", status: domain.StatusApplied, daysAgo: 2000,
			wantVerdict: domain.VerdictDuplicate,
			why:         "preventing a second application to the same job is the whole point",
		},
		{
			name: "rejected never resurfaces", status: domain.StatusRejected, daysAgo: 2000,
			wantVerdict: domain.VerdictDuplicate,
		},
		{
			name: "screened out within cooldown", status: domain.StatusScreenedOut, daysAgo: 60,
			wantVerdict: domain.VerdictDuplicate,
		},
		{
			name: "screened out past cooldown", status: domain.StatusScreenedOut, daysAgo: 120,
			wantVerdict: domain.VerdictEligibleAgain,
			why:         "a role skipped months ago may fit now, and the caller should get the chance",
		},
		{
			name: "never judged, past its short cooldown", status: domain.StatusDiscovered, daysAgo: 45,
			wantVerdict: domain.VerdictEligibleAgain,
		},
		{
			name: "ghosted past cooldown", status: domain.StatusGhosted, daysAgo: 150,
			wantVerdict: domain.VerdictEligibleAgain,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine, store, tenantID := newEngine(t)
			store.seed(t, "Acme, Inc.", domain.Candidate{
				URL:   "https://boards.greenhouse.io/acme/jobs/4012345",
				Title: "Senior Software Engineer II",
			}, tc.status, now.AddDate(0, 0, -tc.daysAgo))

			got := filterOne(t, engine, tenantID, domain.Candidate{
				URL:     "https://boards.greenhouse.io/acme/jobs/4012345",
				Title:   "Senior Software Engineer II",
				Company: "Acme",
			})

			assert.Equal(t, tc.wantVerdict, got.Verdict, tc.why)
			assert.Equal(t, domain.TierExactURL, got.Tier,
				"the match itself is unchanged; only the policy decides what to do about it")
		})
	}
}

func TestEngine_CandidatesCanDuplicateEachOtherWithinOneRequest(t *testing.T) {
	engine, _, tenantID := newEngine(t)

	results, err := engine.Filter(t.Context(), tenantID, []domain.Candidate{
		{URL: "https://boards.greenhouse.io/acme/jobs/4012345", Title: "Senior Software Engineer II", Company: "Acme"},
		{URL: "https://boards.greenhouse.io/acme/jobs/4012345?utm_source=x", Title: "Senior Software Engineer II", Company: "Acme"},
		{URL: "https://www.linkedin.com/jobs/view/999", Title: "Sr. SWE II", Company: "Acme, Inc."},
		{URL: "https://boards.greenhouse.io/globex/jobs/7", Title: "Data Engineer", Company: "Globex"},
	}, domain.DefaultResurfacePolicy())
	require.NoError(t, err)
	require.Len(t, results, 4)

	assert.Equal(t, domain.VerdictNew, results[0].Verdict)
	assert.Equal(t, domain.VerdictDuplicate, results[1].Verdict, "same canonical url as candidate 0")
	assert.Contains(t, results[1].Reason, "candidate 0")
	assert.Equal(t, domain.VerdictDuplicate, results[2].Verdict, "same fingerprint as candidate 0")
	assert.Equal(t, domain.TierFingerprint, results[2].Tier)
	assert.Equal(t, domain.VerdictNew, results[3].Verdict)
}

func TestEngine_ResultsAreIndexedToTheRequest(t *testing.T) {
	engine, _, tenantID := newEngine(t)

	results, err := engine.Filter(t.Context(), tenantID, []domain.Candidate{
		{URL: "https://a.example/1", Title: "Engineer", Company: "A"},
		{URL: "https://b.example/2", Title: "Engineer", Company: "B"},
		{URL: "https://c.example/3", Title: "Engineer", Company: "C"},
	}, domain.DefaultResurfacePolicy())
	require.NoError(t, err)

	for i, r := range results {
		assert.Equal(t, i, r.Index, "callers correlate by position, so index must always be set")
	}
}

func TestEngine_EmptyBatch(t *testing.T) {
	engine, _, tenantID := newEngine(t)
	results, err := engine.Filter(t.Context(), tenantID, nil, domain.DefaultResurfacePolicy())
	require.NoError(t, err)
	assert.Empty(t, results)
}
