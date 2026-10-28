package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// maxBatchCandidates bounds a single request. It is generous enough for a full
// page of search results and small enough that one call cannot monopolise the
// database.
const maxBatchCandidates = 500

// FilterRequest asks which of these candidates are worth pursuing.
type FilterRequest struct {
	// ProfileName selects the search profile whose resurfacing rules apply.
	// Empty means the built-in defaults.
	ProfileName string
	// ResurfaceOverride replaces the profile's rules for this call only.
	ResurfaceOverride *domain.ResurfacePolicy
	Candidates        []domain.Candidate
	// SearchRunID, when set, accumulates this batch's counts onto that run.
	SearchRunID *domain.ID
}

// Summary counts verdicts, so a caller can log one line instead of tallying.
type Summary struct {
	Total         int `json:"total"`
	New           int `json:"new"`
	Duplicate     int `json:"duplicate"`
	EligibleAgain int `json:"eligible_again"`
	NeedsReview   int `json:"needs_review"`
}

// FilterResponse is the answer to a batch dedupe request.
type FilterResponse struct {
	Results []domain.FilterResult
	Summary Summary
	// ProfileApplied names the profile whose policy was used, or "" for the
	// defaults. Echoing it back removes any doubt about which rules ran.
	ProfileApplied string
}

// FilterPostings reports which candidates the tenant has already seen.
//
// It is read-only by design. A search agent can therefore run with a key that
// holds only the read scope and still do the thing it exists to do, and a
// caller can safely retry a filter that timed out.
func (s *Service) FilterPostings(ctx context.Context, p domain.Principal, req FilterRequest) (FilterResponse, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return FilterResponse{}, err
	}
	if err := validateCandidates(req.Candidates); err != nil {
		return FilterResponse{}, err
	}

	policy, profileName, err := s.resolvePolicy(ctx, s.store, p.TenantID, req.ProfileName, req.ResurfaceOverride)
	if err != nil {
		return FilterResponse{}, err
	}

	results, err := s.matcherFor(s.store).Filter(ctx, p.TenantID, req.Candidates, policy)
	if err != nil {
		return FilterResponse{}, fmt.Errorf("filter candidates: %w", err)
	}

	summary := summarise(results)
	if req.SearchRunID != nil {
		// Recording progress must not fail the filter: the counts are an audit
		// convenience, and the caller already has the answer it asked for.
		s.recordRunProgress(ctx, p.TenantID, *req.SearchRunID, summary)
	}

	return FilterResponse{Results: results, Summary: summary, ProfileApplied: profileName}, nil
}

// IngestRequest records candidates the caller wants remembered.
type IngestRequest struct {
	ProfileName       string
	ResurfaceOverride *domain.ResurfacePolicy
	Candidates        []domain.Candidate
	SearchRunID       *domain.ID
}

// IngestResult is what happened to one candidate.
type IngestResult struct {
	domain.FilterResult
	// Created distinguishes a posting recorded for the first time from one
	// that was already known and has merely been seen again.
	Created bool
}

// IngestResponse is the answer to a batch ingest.
type IngestResponse struct {
	Results        []IngestResult
	Summary        Summary
	ProfileApplied string
}

// IngestPostings deduplicates and then records candidates.
//
// The whole batch runs in one transaction, and the match engine reads through
// that transaction. Without that, two candidates in the same batch that
// duplicate each other would both look new and the second insert would fail on
// a uniqueness constraint.
//
// Candidates that come back needs_review are deliberately not recorded. The
// engine could not tell whether they duplicate something, and guessing either
// way is worse than handing the decision back.
func (s *Service) IngestPostings(ctx context.Context, p domain.Principal, req IngestRequest) (IngestResponse, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return IngestResponse{}, err
	}
	if err := validateCandidates(req.Candidates); err != nil {
		return IngestResponse{}, err
	}

	var resp IngestResponse
	err := s.store.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		policy, profileName, err := s.resolvePolicy(ctx, tx, p.TenantID, req.ProfileName, req.ResurfaceOverride)
		if err != nil {
			return err
		}
		resp.ProfileApplied = profileName

		verdicts, err := s.matcherFor(tx).Filter(ctx, p.TenantID, req.Candidates, policy)
		if err != nil {
			return fmt.Errorf("filter candidates: %w", err)
		}

		resp.Results = make([]IngestResult, len(verdicts))
		for i, verdict := range verdicts {
			result, err := s.applyIngest(ctx, tx, p, req.Candidates[i], verdict)
			if err != nil {
				return err
			}
			resp.Results[i] = result
		}
		return nil
	})
	if err != nil {
		return IngestResponse{}, err
	}

	filterResults := make([]domain.FilterResult, len(resp.Results))
	for i, r := range resp.Results {
		filterResults[i] = r.FilterResult
	}
	resp.Summary = summarise(filterResults)

	if req.SearchRunID != nil {
		s.recordRunProgress(ctx, p.TenantID, *req.SearchRunID, resp.Summary)
	}
	return resp, nil
}

// applyIngest turns one verdict into a database change.
func (s *Service) applyIngest(ctx context.Context, tx store.Store, p domain.Principal, candidate domain.Candidate, verdict domain.FilterResult) (IngestResult, error) {
	now := s.now()

	switch verdict.Verdict {
	case domain.VerdictNew:
		posting, application, err := s.createPosting(ctx, tx, p, candidate, now)
		if err != nil {
			return IngestResult{}, err
		}
		verdict.PostingID = &posting.ID
		verdict.ApplicationID = &application.ID
		verdict.ApplicationStatus = &application.Status
		verdict.FirstSeenAt = &posting.FirstSeenAt
		verdict.LastSeenAt = &posting.LastSeenAt
		return IngestResult{FilterResult: verdict, Created: true}, nil

	case domain.VerdictDuplicate, domain.VerdictEligibleAgain:
		if verdict.PostingID == nil {
			// An intra-batch duplicate: the candidate it duplicates was
			// recorded moments ago in this same transaction, so there is
			// nothing further to do.
			return IngestResult{FilterResult: verdict}, nil
		}
		posting, err := tx.Postings().TouchSeen(ctx, p.TenantID, *verdict.PostingID, now)
		if err != nil {
			return IngestResult{}, err
		}
		verdict.LastSeenAt = &posting.LastSeenAt

		if verdict.ApplicationID != nil {
			// Record that the posting resurfaced. This is what makes "still
			// listed after six weeks" answerable later.
			if err := s.appendEvent(ctx, tx, p, *verdict.ApplicationID, domain.ApplicationEvent{
				Type:       domain.EventRediscovered,
				OccurredAt: now,
				Payload:    jsonPayload(map[string]any{"tier": verdict.Tier, "verdict": verdict.Verdict}),
			}); err != nil {
				return IngestResult{}, err
			}
		}
		return IngestResult{FilterResult: verdict}, nil

	default: // needs_review
		return IngestResult{FilterResult: verdict}, nil
	}
}

// createPosting records a company, a posting, an application, and the opening
// event of its timeline.
func (s *Service) createPosting(ctx context.Context, tx store.Store, p domain.Principal, candidate domain.Candidate, now time.Time) (domain.Posting, domain.Application, error) {
	n := dedupe.Normalize(candidate)

	displayName := candidate.Company
	if displayName == "" {
		displayName = n.Identity.CompanySlug
	}
	if n.Company == "" {
		return domain.Posting{}, domain.Application{}, domain.NewValidationError(
			"cannot record a posting with no identifiable company: supply company, or a url from a supported job board")
	}
	if n.Title == "" {
		// A titleless posting can never be matched again: tiers 3 and 5 both
		// skip a candidate with no title, on purpose, because a fingerprint
		// or a similarity score built from nothing would match everything.
		// Recording one anyway creates a posting no future sighting of the
		// same job can ever be recognised against — and, since the
		// fingerprint index is keyed on (company, title, location), a second
		// title-less posting at the same company collides with it outright.
		return domain.Posting{}, domain.Application{}, domain.NewValidationError(
			"cannot record a posting with no identifiable title: supply title, or a url whose page title dedupe can read")
	}

	company, err := tx.Companies().Upsert(ctx, domain.Company{
		TenantID:       p.TenantID,
		Name:           displayName,
		NormalizedName: n.Company,
	})
	if err != nil {
		return domain.Posting{}, domain.Application{}, err
	}

	posting, err := tx.Postings().Insert(ctx, domain.Posting{
		TenantID:        p.TenantID,
		CompanyID:       company.ID,
		Source:          n.Source(),
		ExternalID:      n.ExternalID(),
		CanonicalURL:    n.CanonicalURL,
		RawURL:          candidate.URL,
		Title:           candidate.Title,
		NormalizedTitle: n.Title,
		LocationRaw:     candidate.Location,
		LocationBucket:  n.LocationBucket,
		RemoteType:      candidate.RemoteType,
		Seniority:       candidate.Seniority,
		Compensation:    candidate.Compensation,
		VisaSponsorship: candidate.VisaSponsorship,
		TechTags:        candidate.TechTags,
		DescriptionHash: n.DescriptionHash,
		Fingerprint:     n.Fingerprint,
		PostedAt:        candidate.PostedAt,
		FirstSeenAt:     now,
		Raw:             candidate.Raw,
	})
	if err != nil {
		return domain.Posting{}, domain.Application{}, err
	}

	application, err := tx.Applications().Insert(ctx, domain.Application{
		TenantID:  p.TenantID,
		PostingID: posting.ID,
		Status:    domain.StatusDiscovered,
	})
	if err != nil {
		return domain.Posting{}, domain.Application{}, err
	}

	discovered := domain.StatusDiscovered
	if err := s.appendEvent(ctx, tx, p, application.ID, domain.ApplicationEvent{
		Type:       domain.EventCreated,
		ToStatus:   &discovered,
		OccurredAt: now,
		Payload:    jsonPayload(map[string]any{"source": string(n.Source()), "url": n.CanonicalURL}),
	}); err != nil {
		return domain.Posting{}, domain.Application{}, err
	}

	return posting, application, nil
}

// appendEvent writes one timeline entry, filling in the tenant, application,
// and actor so callers cannot forget them.
func (s *Service) appendEvent(ctx context.Context, tx store.Store, p domain.Principal, applicationID domain.ID, event domain.ApplicationEvent) error {
	event.TenantID = p.TenantID
	event.ApplicationID = applicationID
	event.Actor = actorOf(p)
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now()
	}
	_, err := tx.Events().Insert(ctx, event)
	return err
}

// recordRunProgress is best-effort bookkeeping; a failure here must not fail
// the caller's actual request. It is not, however, a failure worth hiding
// entirely: a wrong or already-finished search_run_id is a caller mistake
// worth a log line, even though the filter/ingest result the caller actually
// asked for is unaffected.
func (s *Service) recordRunProgress(ctx context.Context, tenantID, runID domain.ID, summary Summary) {
	_, err := s.store.SearchRuns().RecordProgress(context.WithoutCancel(ctx), tenantID, runID, store.SearchRunProgress{
		CandidatesSeen:     summary.Total,
		NewCount:           summary.New,
		EligibleAgainCount: summary.EligibleAgain,
		DuplicateCount:     summary.Duplicate,
		NeedsReviewCount:   summary.NeedsReview,
	})
	if err != nil {
		slog.Warn("search run progress not recorded",
			"search_run_id", runID, "tenant_id", tenantID, "error", err)
	}
}

func validateCandidates(candidates []domain.Candidate) error {
	if len(candidates) == 0 {
		return domain.NewValidationError("candidates must not be empty")
	}
	if len(candidates) > maxBatchCandidates {
		return domain.NewValidationError(
			fmt.Sprintf("a batch may hold at most %d candidates, got %d", maxBatchCandidates, len(candidates)))
	}

	var v domain.ValidationError
	for i, c := range candidates {
		if err := c.Validate(); err != nil {
			var invalid *domain.ValidationError
			if ok := asValidation(err, &invalid); ok {
				for _, problem := range invalid.Details() {
					v.Add("candidates[%d]: %s", i, problem)
				}
				continue
			}
			// Validate() only ever returns nil or *ValidationError today, but
			// silently accepting an error of any other shape would let a
			// future validation rule pass candidates it meant to reject.
			v.Add("candidates[%d]: %s", i, err.Error())
		}
	}
	return v.OrNil()
}

func summarise(results []domain.FilterResult) Summary {
	summary := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Verdict {
		case domain.VerdictNew:
			summary.New++
		case domain.VerdictDuplicate:
			summary.Duplicate++
		case domain.VerdictEligibleAgain:
			summary.EligibleAgain++
		case domain.VerdictNeedsReview:
			summary.NeedsReview++
		}
	}
	return summary
}

func jsonPayload(v map[string]any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		// The payload is diagnostic metadata built from known-safe values; a
		// marshalling failure must not sink the write that carries it.
		return []byte(`{}`)
	}
	return raw
}
