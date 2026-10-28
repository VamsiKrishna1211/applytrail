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

// These five pairs are drawn from a real ApplyTrail deployment's data, where
// the engine missed every one of them: BucketLocation reduced enough of the
// location text to noise that the fingerprint and the fuzzy tier's location
// filter both saw two different jobs. Each subtest is one pair, in the shape
// it actually occurred.
func TestEngine_Tier3And5_ProductionLocationMisses(t *testing.T) {
	t.Run("a specific city and a remote label for the same posting", func(t *testing.T) {
		// Blue River Technology, "Machine Learning Engineer, Autonomy": one
		// listing named Sunnyvale, the other just said "US, Remote".
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Blue River Technology", domain.Candidate{
			Title:    "Machine Learning Engineer, Autonomy",
			Location: "Sunnyvale, CA",
		}, domain.StatusDiscovered, now.AddDate(0, 0, -1))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://jobs.khoslaventures.com/companies/blue-river-technology-2/jobs/90277310",
			Title:    "Machine Learning Engineer, Autonomy",
			Company:  "Blue River Technology",
			Location: "US, Remote",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	})

	t.Run("a placeholder location must not block a real match", func(t *testing.T) {
		// CircuitHub, "Computer Vision Engineer": one listing said "Remote
		// (US/UK)", the other said only "Not stated".
		engine, store, tenantID := newEngine(t)
		store.seed(t, "CircuitHub", domain.Candidate{
			Title:    "Computer Vision Engineer (Founding CV Team)",
			Location: "Remote (US/UK)",
		}, domain.StatusDiscovered, now.AddDate(0, 0, -1))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://ycombinator.com/companies/circuithub/jobs/js2U2cL-computer-vision-engineer",
			Title:    "Computer Vision Engineer",
			Company:  "CircuitHub",
			Location: "Not stated",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	})

	t.Run("a placeholder location on the candidate side, matched via the fuzzy tier", func(t *testing.T) {
		// Niantic Spatial, "Senior Computer Vision Engineer": the parenthetical
		// (Reconstruction) vs (Localization) is stripped by title
		// normalisation either way, so only the location stood between this
		// pair and a match. "Not specified" carries no bucket at all, so tier
		// 3's exact fingerprint still misses it (there is nothing to hash
		// against "san francisco"), but tier 5 treats an unknown location as
		// compatible with anything and resolves it on title alone.
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Niantic Spatial", domain.Candidate{
			Title:    "Senior Computer Vision Engineer (Reconstruction)",
			Location: "San Francisco, CA / Sunnyvale, CA",
		}, domain.StatusDiscovered, now.AddDate(0, 0, -1))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://jobs.ashbyhq.com/niantic-spatial/5f63e446-b7d0-4c75-8921-f9512d28e4c1",
			Title:    "Senior Computer Vision Engineer (Localization)",
			Company:  "Niantic Spatial",
			Location: "Not specified",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
		assert.Equal(t, domain.TierFuzzy, got.Tier)
	})

	t.Run("a multi-site listing must match a later single-site sighting of any of its sites", func(t *testing.T) {
		// Treeswift, "Robotics Engineer (Perception)": one listing named only
		// Philadelphia, the other listed three cities including it.
		engine, store, tenantID := newEngine(t)
		store.seed(t, "Treeswift", domain.Candidate{
			Title:    "Robotics Engineer (Perception)",
			Location: "Philadelphia, PA",
		}, domain.StatusDiscovered, now.AddDate(0, 0, -1))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://jobs.ashbyhq.com/treeswift/2048138a-8e0f-48f5-b04d-f0159b610c29",
			Title:    "Robotics Engineer (Perception)",
			Company:  "Treeswift",
			Location: "Manhattan/SF/Philadelphia",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
		assert.Equal(t, domain.TierFingerprint, got.Tier)
	})

	t.Run("a parenthetical aside on the location must not create a new bucket", func(t *testing.T) {
		// World Labs, "Research Engineer / Scientist (SLAM)": one listing
		// said "San Francisco, CA", the other appended "(relocation required)".
		engine, store, tenantID := newEngine(t)
		store.seed(t, "World Labs", domain.Candidate{
			Title:    "Research Engineer / Scientist (SLAM)",
			Location: "San Francisco, CA",
		}, domain.StatusDiscovered, now.AddDate(0, 0, -1))

		got := filterOne(t, engine, tenantID, domain.Candidate{
			URL:      "https://job-boards.greenhouse.io/worldlabs/jobs/4135311009",
			Title:    "Research Engineer / Scientist (SLAM)",
			Company:  "World Labs",
			Location: "San Francisco (relocation required)",
		})
		assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
		assert.Equal(t, domain.TierFingerprint, got.Tier)
	})
}

// Tier 4 must not cross a company boundary: two different employers reusing
// the same agency boilerplate are not the same job. Globex has a posting of
// its own on record (a different description) so its company row exists and
// the tier 4 lookup actually runs scoped to it, rather than trivially
// skipping because "an employer with no postings on record cannot have a
// duplicate" — this is what proves the company_id filter itself, not just
// the absence of a Globex row.
func TestEngine_Tier4_ScopedToOneCompany(t *testing.T) {
	engine, store, tenantID := newEngine(t)

	description := "We are hiring an engineer to design and operate distributed systems " +
		"serving millions of requests per second. You will own services end to end, " +
		"from design review through production operation, and mentor other engineers " +
		"on the platform team. This posting was placed through a staffing agency."

	store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:         "https://boards.greenhouse.io/acme/jobs/1",
		Title:       "Distributed Systems Engineer",
		Description: description,
	}, domain.StatusDiscovered, now.AddDate(0, 0, -2))
	store.seed(t, "Globex", domain.Candidate{
		URL:         "https://boards.greenhouse.io/globex/jobs/2",
		Title:       "Unrelated Sales Role",
		Description: "A completely different posting, long enough to hash but sharing no text with the engineering one above at all, padded out well past the two hundred character minimum so it is actually eligible for tier four hashing in the first place.",
	}, domain.StatusDiscovered, now.AddDate(0, 0, -2))

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:         "https://boards.greenhouse.io/globex/jobs/9",
		Title:       "Platform Infrastructure Specialist",
		Company:     "Globex",
		Description: description,
	})

	assert.Equal(t, domain.VerdictNew, got.Verdict,
		"identical boilerplate at a different employer must not be reported as the same job")
}

// A case that genuinely reaches the fuzzy tier's duplicate branch, not tier
// 3: the normalised title is identical (so the score is 1.0, comfortably
// past MatchThreshold), but the fingerprint still cannot match because the
// two locations hash to different buckets ("remote" vs a specific city) with
// no bucket in common. Tier 5's location check treats "remote" as compatible
// with anything, which is what lets this resolve instead of being reported
// new.
func TestEngine_Tier5_FuzzyTitleReachesDuplicateVerdict(t *testing.T) {
	engine, store, tenantID := newEngine(t)
	store.seed(t, "Acme, Inc.", domain.Candidate{
		URL:      "https://boards.greenhouse.io/acme/jobs/1",
		Title:    "Senior Software Engineer II",
		Location: "Remote",
	}, domain.StatusApplied, now.AddDate(0, 0, -5))

	got := filterOne(t, engine, tenantID, domain.Candidate{
		URL:      "https://acme.com/careers/9",
		Title:    "Sr. SWE II",
		Company:  "Acme",
		Location: "New York, NY",
	})
	assert.Equal(t, domain.VerdictDuplicate, got.Verdict)
	assert.Equal(t, domain.TierFuzzy, got.Tier,
		"tier 3 must not have already resolved this via the fingerprint")
	assert.Equal(t, 1.0, got.Confidence)
}
