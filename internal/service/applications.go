package service

import (
	"context"
	"fmt"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// RecordApplicationRequest marks a posting as applied to.
//
// Either PostingID or Candidate must be set. Accepting an inline candidate
// matters: an agent that applies to a job it found seconds ago should not have
// to ingest it first and then reference the ID back.
type RecordApplicationRequest struct {
	PostingID *domain.ID
	Candidate *domain.Candidate

	AppliedAt          *time.Time
	ResumeVariant      string
	ResumeURL          string
	ResumeHash         string
	CoverLetterVariant string
	Notes              string
	// FollowUpInDays schedules the next nudge. Zero uses the default SLA.
	FollowUpInDays int
}

// defaultFollowUpDays is how long to wait before an application with no reply
// is worth chasing.
const defaultFollowUpDays = 14

// RecordApplication moves a posting to applied, creating it first if the
// caller supplied one inline.
func (s *Service) RecordApplication(ctx context.Context, p domain.Principal, req RecordApplicationRequest) (domain.Application, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.Application{}, err
	}
	if req.PostingID == nil && req.Candidate == nil {
		return domain.Application{}, domain.NewValidationError("either posting_id or a posting must be supplied")
	}

	appliedAt := s.now()
	if req.AppliedAt != nil {
		appliedAt = *req.AppliedAt
	}
	followUpDays := req.FollowUpInDays
	if followUpDays <= 0 {
		followUpDays = defaultFollowUpDays
	}
	nextAction := appliedAt.AddDate(0, 0, followUpDays)

	var result domain.Application
	err := s.store.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		postingID, err := s.resolvePosting(ctx, tx, p, req)
		if err != nil {
			return err
		}

		application, err := tx.Applications().GetByPosting(ctx, p.TenantID, postingID)
		if err != nil {
			return err
		}

		if application.Status != domain.StatusApplied {
			if err := domain.ValidateTransition(application.Status, domain.StatusApplied); err != nil {
				return err
			}
			application, err = tx.Applications().ApplyTransition(ctx, p.TenantID, application.ID, store.StatusTransition{
				From:         application.Status,
				To:           domain.StatusApplied,
				AppliedAt:    &appliedAt,
				NextActionAt: &nextAction,
			})
			if err != nil {
				return err
			}
		}

		application, err = tx.Applications().Update(ctx, p.TenantID, application.ID, store.ApplicationUpdate{
			ResumeVariant:      optional(req.ResumeVariant),
			ResumeURL:          optional(req.ResumeURL),
			ResumeHash:         optional(req.ResumeHash),
			CoverLetterVariant: optional(req.CoverLetterVariant),
			Notes:              optional(req.Notes),
		})
		if err != nil {
			return err
		}

		from := domain.StatusDiscovered
		applied := domain.StatusApplied
		if err := s.appendEvent(ctx, tx, p, application.ID, domain.ApplicationEvent{
			Type:       domain.EventStatusChanged,
			FromStatus: &from,
			ToStatus:   &applied,
			OccurredAt: appliedAt,
			Payload:    jsonPayload(map[string]any{"resume_variant": req.ResumeVariant}),
		}); err != nil {
			return err
		}

		result = application
		return nil
	})
	return result, err
}

// resolvePosting returns the posting to act on, ingesting the inline candidate
// when one was supplied.
func (s *Service) resolvePosting(ctx context.Context, tx store.Store, p domain.Principal, req RecordApplicationRequest) (domain.ID, error) {
	if req.PostingID != nil {
		if _, err := tx.Postings().Get(ctx, p.TenantID, *req.PostingID); err != nil {
			return domain.ID{}, err
		}
		return *req.PostingID, nil
	}

	// Deduplicate the inline candidate rather than inserting blindly: the job
	// being applied to is very often one already on record.
	verdicts, err := s.matcherFor(tx).Filter(ctx, p.TenantID, []domain.Candidate{*req.Candidate}, domain.DefaultResurfacePolicy())
	if err != nil {
		return domain.ID{}, fmt.Errorf("match inline posting: %w", err)
	}
	if verdicts[0].PostingID != nil {
		return *verdicts[0].PostingID, nil
	}

	posting, _, err := s.createPosting(ctx, tx, p, *req.Candidate, s.now())
	if err != nil {
		return domain.ID{}, err
	}
	return posting.ID, nil
}

// UpdateStatusRequest advances an application through the pipeline.
type UpdateStatusRequest struct {
	ApplicationID domain.ID
	Status        domain.Status
	OccurredAt    *time.Time
	Note          string
	// NextActionAt schedules the next follow-up.
	NextActionAt *time.Time
	// ClearNextAction removes any scheduled follow-up, which is what a
	// terminal status should do.
	ClearNextAction bool
}

// UpdateStatus applies a transition and records it on the timeline.
func (s *Service) UpdateStatus(ctx context.Context, p domain.Principal, req UpdateStatusRequest) (domain.Application, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.Application{}, err
	}

	occurredAt := s.now()
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}

	var result domain.Application
	err := s.store.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		current, err := tx.Applications().Get(ctx, p.TenantID, req.ApplicationID)
		if err != nil {
			return err
		}
		if err := domain.ValidateTransition(current.Status, req.Status); err != nil {
			return err
		}

		transition := store.StatusTransition{
			From:            current.Status,
			To:              req.Status,
			NextActionAt:    req.NextActionAt,
			ClearNextAction: req.ClearNextAction,
		}
		// Reaching an applied-or-later status without a recorded date would
		// violate the schema's check constraint, so fill it in from the event.
		if req.Status.IsEngaged() && current.AppliedAt == nil {
			transition.AppliedAt = &occurredAt
		}
		// Nothing is left to chase once the pipeline has ended.
		if req.Status.IsTerminal() {
			transition.ClearNextAction = true
		}

		updated, err := tx.Applications().ApplyTransition(ctx, p.TenantID, req.ApplicationID, transition)
		if err != nil {
			return err
		}

		from := current.Status
		to := req.Status
		if err := s.appendEvent(ctx, tx, p, req.ApplicationID, domain.ApplicationEvent{
			Type:       domain.EventStatusChanged,
			FromStatus: &from,
			ToStatus:   &to,
			OccurredAt: occurredAt,
			Payload:    jsonPayload(map[string]any{"note": req.Note}),
		}); err != nil {
			return err
		}

		if req.Note != "" {
			updated, err = tx.Applications().Update(ctx, p.TenantID, req.ApplicationID, store.ApplicationUpdate{
				Notes: optional(appendNote(current.Notes, occurredAt, req.Note)),
			})
			if err != nil {
				return err
			}
		}

		result = updated
		return nil
	})
	return result, err
}

// AddNote appends to an application's notes and records it on the timeline.
func (s *Service) AddNote(ctx context.Context, p domain.Principal, applicationID domain.ID, note string) (domain.Application, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.Application{}, err
	}
	if note == "" {
		return domain.Application{}, domain.NewValidationError("note must not be empty")
	}

	now := s.now()
	var result domain.Application
	err := s.store.Tx(ctx, func(ctx context.Context, tx store.Store) error {
		current, err := tx.Applications().Get(ctx, p.TenantID, applicationID)
		if err != nil {
			return err
		}

		updated, err := tx.Applications().Update(ctx, p.TenantID, applicationID, store.ApplicationUpdate{
			Notes: optional(appendNote(current.Notes, now, note)),
		})
		if err != nil {
			return err
		}

		if err := s.appendEvent(ctx, tx, p, applicationID, domain.ApplicationEvent{
			Type:       domain.EventNoteAdded,
			OccurredAt: now,
			Payload:    jsonPayload(map[string]any{"note": note}),
		}); err != nil {
			return err
		}

		result = updated
		return nil
	})
	return result, err
}

// ListApplications returns a page of the pipeline.
func (s *Service) ListApplications(ctx context.Context, p domain.Principal, filter store.ApplicationListFilter) ([]store.ApplicationWithPosting, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}
	for _, status := range filter.Statuses {
		if !status.Valid() {
			return nil, domain.NewValidationError(fmt.Sprintf("status %q is not recognised", status))
		}
	}
	return s.store.Applications().List(ctx, p.TenantID, filter)
}

// GetApplication returns one application with its timeline and contacts.
type ApplicationDetail struct {
	Application store.ApplicationWithPosting
	Events      []domain.ApplicationEvent
	Contacts    []domain.Contact
}

// GetApplication returns everything known about one application.
func (s *Service) GetApplication(ctx context.Context, p domain.Principal, id domain.ID, eventLimit int) (ApplicationDetail, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return ApplicationDetail{}, err
	}

	application, err := s.store.Applications().Get(ctx, p.TenantID, id)
	if err != nil {
		return ApplicationDetail{}, err
	}
	posting, err := s.store.Postings().Get(ctx, p.TenantID, application.PostingID)
	if err != nil {
		return ApplicationDetail{}, err
	}
	company, err := s.store.Companies().Get(ctx, p.TenantID, posting.CompanyID)
	if err != nil {
		return ApplicationDetail{}, err
	}
	events, err := s.store.Events().List(ctx, p.TenantID, id, eventLimit)
	if err != nil {
		return ApplicationDetail{}, err
	}
	contacts, err := s.store.Contacts().ListForApplication(ctx, p.TenantID, id)
	if err != nil {
		return ApplicationDetail{}, err
	}

	return ApplicationDetail{
		Application: store.ApplicationWithPosting{
			Application:    application,
			PostingTitle:   posting.Title,
			PostingURL:     posting.CanonicalURL,
			LocationBucket: posting.LocationBucket,
			CompanyName:    company.Name,
		},
		Events:   events,
		Contacts: contacts,
	}, nil
}

// FollowUp is an application that has gone quiet for long enough to act on.
type FollowUp struct {
	store.ApplicationWithPosting
	// DaysSinceApplied is precomputed so a caller does not have to do date
	// arithmetic to decide how firmly to nudge.
	DaysSinceApplied int
	// DaysOverdue is how long the scheduled action has been pending.
	DaysOverdue int
}

// GetFollowupsDue returns applications whose next action is due.
func (s *Service) GetFollowupsDue(ctx context.Context, p domain.Principal, asOf *time.Time, limit int) ([]FollowUp, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}

	now := s.now()
	if asOf != nil {
		now = *asOf
	}

	rows, err := s.store.Applications().ListFollowupsDue(ctx, p.TenantID, now, limit)
	if err != nil {
		return nil, err
	}

	followUps := make([]FollowUp, len(rows))
	for i, row := range rows {
		f := FollowUp{ApplicationWithPosting: row}
		if row.Application.AppliedAt != nil {
			f.DaysSinceApplied = daysBetween(*row.Application.AppliedAt, now)
		}
		if row.Application.NextActionAt != nil {
			f.DaysOverdue = daysBetween(*row.Application.NextActionAt, now)
		}
		followUps[i] = f
	}
	return followUps, nil
}

func daysBetween(then, now time.Time) int {
	days := int(now.Sub(then).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// appendNote keeps notes as a dated log rather than overwriting, so context
// accumulates instead of being lost to the most recent edit.
func appendNote(existing string, at time.Time, note string) string {
	entry := at.Format("2006-01-02") + ": " + note
	if existing == "" {
		return entry
	}
	return existing + "\n" + entry
}

// optional converts an empty string to nil, which the store reads as "leave
// this field unchanged".
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
