package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/testsupport"
)

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func newService(t *testing.T) (*service.Service, domain.Principal) {
	t.Helper()
	st, tenantID := testsupport.Store(t)
	svc := service.New(st, service.Options{Now: func() time.Time { return now }})
	return svc, testsupport.Principal(tenantID)
}

func greenhouseCandidate(id, title string) domain.Candidate {
	return domain.Candidate{
		URL:      "https://boards.greenhouse.io/acme/jobs/" + id,
		Title:    title,
		Company:  "Acme, Inc.",
		Location: "Remote",
	}
}

// The whole product in one test: a second run must not re-surface what the
// first run already recorded.
func TestIngestThenFilter_SecondRunFindsNothingNew(t *testing.T) {
	svc, principal := newService(t)
	candidates := []domain.Candidate{
		greenhouseCandidate("4012345", "Senior Software Engineer II"),
		greenhouseCandidate("4012346", "Staff Engineer"),
	}

	ingested, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{Candidates: candidates})
	require.NoError(t, err)
	assert.Equal(t, 2, ingested.Summary.New)
	for _, r := range ingested.Results {
		assert.True(t, r.Created)
	}

	// The same search runs again the next day and returns the same jobs.
	filtered, err := svc.FilterPostings(t.Context(), principal, service.FilterRequest{Candidates: candidates})
	require.NoError(t, err)
	assert.Equal(t, 0, filtered.Summary.New, "nothing here is new any more")
	assert.Equal(t, 2, filtered.Summary.Duplicate)
}

func TestIngest_IsIdempotent(t *testing.T) {
	svc, principal := newService(t)
	candidates := []domain.Candidate{greenhouseCandidate("4012345", "Senior Software Engineer II")}

	first, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{Candidates: candidates})
	require.NoError(t, err)
	require.True(t, first.Results[0].Created)

	// A retried ingest, after a timeout the caller could not distinguish from
	// a failure, must not create a second posting.
	second, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{Candidates: candidates})
	require.NoError(t, err)
	assert.False(t, second.Results[0].Created)
	assert.Equal(t, *first.Results[0].PostingID, *second.Results[0].PostingID)
}

// Two candidates in one batch that are the same job must produce one posting.
// Without the matcher reading through the ingest transaction, the second would
// look new and its insert would collide on the fingerprint constraint.
func TestIngest_CollapsesDuplicatesWithinOneBatch(t *testing.T) {
	svc, principal := newService(t)

	resp, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{Candidates: []domain.Candidate{
		{URL: "https://boards.greenhouse.io/acme/jobs/4012345", Title: "Senior Software Engineer II", Company: "Acme, Inc.", Location: "Remote"},
		{URL: "https://www.linkedin.com/jobs/view/998877", Title: "Sr. SWE II", Company: "Acme", Location: "Remote - US"},
	}})
	require.NoError(t, err)

	assert.True(t, resp.Results[0].Created)
	assert.False(t, resp.Results[1].Created, "the LinkedIn listing is the same job")
	assert.Equal(t, domain.VerdictDuplicate, resp.Results[1].Verdict)
}

func TestIngest_NeedsReviewIsNotRecorded(t *testing.T) {
	svc, principal := newService(t)

	_, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{
		Candidates: []domain.Candidate{greenhouseCandidate("1111111", "Senior Software Engineer")},
	})
	require.NoError(t, err)

	resp, err := svc.IngestPostings(t.Context(), principal, service.IngestRequest{
		Candidates: []domain.Candidate{{
			URL: "https://acme.com/careers/payments", Title: "Senior Software Engineer, Payments",
			Company: "Acme", Location: "Remote",
		}},
	})
	require.NoError(t, err)

	require.Equal(t, domain.VerdictNeedsReview, resp.Results[0].Verdict)
	assert.False(t, resp.Results[0].Created,
		"the engine could not tell whether this duplicates the existing role, and guessing either way is worse than asking")
}

func TestApplicationLifecycle(t *testing.T) {
	svc, principal := newService(t)
	ctx := t.Context()

	ingested, err := svc.IngestPostings(ctx, principal, service.IngestRequest{
		Candidates: []domain.Candidate{greenhouseCandidate("4012345", "Senior Software Engineer II")},
	})
	require.NoError(t, err)
	postingID := *ingested.Results[0].PostingID

	applied, err := svc.RecordApplication(ctx, principal, service.RecordApplicationRequest{
		PostingID:     &postingID,
		ResumeVariant: "backend-v3",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApplied, applied.Status)
	require.NotNil(t, applied.AppliedAt)
	require.NotNil(t, applied.NextActionAt, "applying schedules the follow-up that makes the queue work")
	assert.Equal(t, "backend-v3", applied.ResumeVariant)

	screening, err := svc.UpdateStatus(ctx, principal, service.UpdateStatusRequest{
		ApplicationID: applied.ID,
		Status:        domain.StatusScreening,
		Note:          "recruiter call booked",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusScreening, screening.Status)
	assert.Contains(t, screening.Notes, "recruiter call booked")

	detail, err := svc.GetApplication(ctx, principal, applied.ID, 50)
	require.NoError(t, err)
	assert.Equal(t, "Acme, Inc.", detail.Application.CompanyName)
	assert.GreaterOrEqual(t, len(detail.Events), 3,
		"created, applied, and screening must all be on the timeline")

	t.Run("an invalid transition is refused", func(t *testing.T) {
		_, err := svc.UpdateStatus(ctx, principal, service.UpdateStatusRequest{
			ApplicationID: applied.ID,
			Status:        domain.StatusDiscovered,
		})
		require.ErrorIs(t, err, domain.ErrInvalidArgument)
	})

	t.Run("a terminal status clears the follow-up", func(t *testing.T) {
		rejected, err := svc.UpdateStatus(ctx, principal, service.UpdateStatusRequest{
			ApplicationID: applied.ID,
			Status:        domain.StatusRejected,
		})
		require.NoError(t, err)
		assert.Nil(t, rejected.NextActionAt, "there is nothing left to chase")
	})
}

func TestRecordApplication_AcceptsAnInlinePosting(t *testing.T) {
	svc, principal := newService(t)

	// An agent that just applied should not have to ingest first and then
	// reference the id back.
	applied, err := svc.RecordApplication(t.Context(), principal, service.RecordApplicationRequest{
		Candidate:     ptr(greenhouseCandidate("4012345", "Senior Software Engineer II")),
		ResumeVariant: "backend-v3",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApplied, applied.Status)

	// And the job it applied to must now be recognised as seen.
	filtered, err := svc.FilterPostings(t.Context(), principal, service.FilterRequest{
		Candidates: []domain.Candidate{greenhouseCandidate("4012345", "Senior Software Engineer II")},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.VerdictDuplicate, filtered.Results[0].Verdict)
	assert.Equal(t, domain.StatusApplied, *filtered.Results[0].ApplicationStatus)
}

func TestFollowupsDue(t *testing.T) {
	svc, principal := newService(t)
	ctx := t.Context()

	applied, err := svc.RecordApplication(ctx, principal, service.RecordApplicationRequest{
		Candidate:      ptr(greenhouseCandidate("4012345", "Senior Software Engineer II")),
		FollowUpInDays: 14,
	})
	require.NoError(t, err)

	due, err := svc.GetFollowupsDue(ctx, principal, ptr(now.AddDate(0, 0, 7)), 10)
	require.NoError(t, err)
	assert.Empty(t, due, "seven days in, there is nothing to chase yet")

	due, err = svc.GetFollowupsDue(ctx, principal, ptr(now.AddDate(0, 0, 21)), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, applied.ID, due[0].Application.ID)
	assert.Equal(t, 21, due[0].DaysSinceApplied, "the caller gets the arithmetic done for it")
	assert.Equal(t, 7, due[0].DaysOverdue)
}

func TestScopesAreEnforced(t *testing.T) {
	st, tenantID := testsupport.Store(t)
	svc := service.New(st, service.Options{Now: func() time.Time { return now }})
	readOnly := testsupport.Principal(tenantID, domain.ScopeRead)

	candidates := []domain.Candidate{greenhouseCandidate("4012345", "Senior Software Engineer II")}

	_, err := svc.FilterPostings(t.Context(), readOnly, service.FilterRequest{Candidates: candidates})
	require.NoError(t, err, "batch dedupe is read-only so a search agent needs no write access")

	_, err = svc.IngestPostings(t.Context(), readOnly, service.IngestRequest{Candidates: candidates})
	require.ErrorIs(t, err, domain.ErrPermissionDenied)

	_, err = svc.CreateAPIKey(t.Context(), readOnly, "x", []domain.Scope{domain.ScopeRead}, nil)
	require.ErrorIs(t, err, domain.ErrPermissionDenied)
}

func TestSearchProfilePolicyOverridesDefaults(t *testing.T) {
	st, tenantID := testsupport.Store(t)
	principal := testsupport.Principal(tenantID)
	svc := service.New(st, service.Options{Now: func() time.Time { return now }})
	ctx := t.Context()

	_, err := svc.CreateProfile(ctx, principal, domain.SearchProfile{
		Name:     "aggressive",
		Active:   true,
		Keywords: []string{"golang"},
		ResurfacePolicy: domain.ResurfacePolicy{CooldownDays: map[domain.Status]int{
			domain.StatusDiscovered: 1,
		}},
	})
	require.NoError(t, err)

	candidates := []domain.Candidate{greenhouseCandidate("4012345", "Senior Software Engineer II")}
	_, err = svc.IngestPostings(ctx, principal, service.IngestRequest{Candidates: candidates})
	require.NoError(t, err)

	// The default cooldown for a never-judged posting is 30 days, so under the
	// defaults this is still a duplicate today.
	def, err := svc.FilterPostings(ctx, principal, service.FilterRequest{Candidates: candidates})
	require.NoError(t, err)
	assert.Equal(t, domain.VerdictDuplicate, def.Results[0].Verdict)

	// Under the profile's one-day cooldown it is eligible again, because the
	// posting was last seen before "now".
	svcLater := service.New(st, service.Options{Now: func() time.Time { return now.AddDate(0, 0, 3) }})
	withProfile, err := svcLater.FilterPostings(ctx, principal, service.FilterRequest{
		ProfileName: "aggressive",
		Candidates:  candidates,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.VerdictEligibleAgain, withProfile.Results[0].Verdict)
	assert.Equal(t, "aggressive", withProfile.ProfileApplied, "the response says which rules ran")
}

func TestStats(t *testing.T) {
	svc, principal := newService(t)
	ctx := t.Context()

	// Each entry must be a genuinely distinct job: identical titles at one
	// company would be deduplicated, which is the service working correctly
	// but not what this test is measuring.
	sent := []struct {
		jobID   string
		title   string
		variant string
		replied bool
	}{
		{jobID: "4000001", title: "Senior Backend Engineer", variant: "backend-v3", replied: true},
		{jobID: "4000002", title: "Staff Platform Engineer", variant: "backend-v3"},
		{jobID: "4000003", title: "Data Engineer", variant: "generic-v1"},
	}

	for _, s := range sent {
		applied, err := svc.RecordApplication(ctx, principal, service.RecordApplicationRequest{
			Candidate:     ptr(greenhouseCandidate(s.jobID, s.title)),
			ResumeVariant: s.variant,
		})
		require.NoError(t, err)

		if s.replied {
			_, err = svc.UpdateStatus(ctx, principal, service.UpdateStatusRequest{
				ApplicationID: applied.ID, Status: domain.StatusScreening,
			})
			require.NoError(t, err)
		}
	}

	stats, err := svc.GetStats(ctx, principal)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalApplied)
	assert.Len(t, stats.ByStatus, len(domain.AllStatuses),
		"every status is reported, including the empty ones, so a caller need not know the list")

	byVariant := map[string]service.ResumeVariantStat{}
	for _, v := range stats.ResumeVariants {
		byVariant[v.Variant] = v
	}
	assert.Equal(t, 2, byVariant["backend-v3"].Sent)
	assert.InDelta(t, 0.5, byVariant["backend-v3"].ResponseRate, 0.001,
		"comparing response rates across resumes is what the variant field is for")
	assert.Zero(t, byVariant["generic-v1"].ResponseRate)
}

func TestValidation(t *testing.T) {
	svc, principal := newService(t)

	_, err := svc.FilterPostings(t.Context(), principal, service.FilterRequest{})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = svc.IngestPostings(t.Context(), principal, service.IngestRequest{
		Candidates: []domain.Candidate{{Title: "Engineer"}},
	})
	require.ErrorIs(t, err, domain.ErrInvalidArgument, "a candidate with no url and no company cannot be identified")
}

func TestListApplications(t *testing.T) {
	svc, principal := newService(t)
	ctx := t.Context()

	_, err := svc.IngestPostings(ctx, principal, service.IngestRequest{Candidates: []domain.Candidate{
		greenhouseCandidate("4012345", "Senior Software Engineer II"),
		greenhouseCandidate("4012346", "Staff Engineer"),
	}})
	require.NoError(t, err)

	all, err := svc.ListApplications(ctx, principal, store.ApplicationListFilter{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, all, 2)
	assert.NotEmpty(t, all[0].CompanyName, "listings carry enough detail to act on without a second lookup")

	none, err := svc.ListApplications(ctx, principal, store.ApplicationListFilter{
		Statuses: []domain.Status{domain.StatusApplied}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = svc.ListApplications(ctx, principal, store.ApplicationListFilter{
		Statuses: []domain.Status{domain.Status("nonsense")},
	})
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
}

func ptr[T any](v T) *T { return &v }
