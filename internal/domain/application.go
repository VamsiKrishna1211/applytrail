package domain

import (
	"fmt"
	"time"
)

// Status is where a posting sits in the tenant's pipeline.
//
// Two statuses mean "not pursued" for different reasons, and the distinction
// drives resurfacing: StatusDiscovered was never judged, StatusScreenedOut was
// judged and rejected by the tenant.
type Status string

const (
	StatusDiscovered  Status = "discovered"
	StatusScreenedOut Status = "screened_out"
	StatusQueued      Status = "queued"
	StatusApplied     Status = "applied"
	StatusScreening   Status = "screening"
	StatusInterview   Status = "interview"
	StatusOffer       Status = "offer"
	StatusAccepted    Status = "accepted"
	StatusRejected    Status = "rejected"
	StatusGhosted     Status = "ghosted"
	StatusWithdrawn   Status = "withdrawn"
)

// AllStatuses lists every status in rough pipeline order. Used for validation
// messages and for the funnel report.
var AllStatuses = []Status{
	StatusDiscovered, StatusScreenedOut, StatusQueued, StatusApplied,
	StatusScreening, StatusInterview, StatusOffer, StatusAccepted,
	StatusRejected, StatusGhosted, StatusWithdrawn,
}

// Valid reports whether s is a recognised status.
func (s Status) Valid() bool {
	_, ok := allowedTransitions[s]
	return ok
}

// IsTerminal reports whether the pipeline has ended for this application.
// Terminal statuses never resurface under the default policy.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusAccepted, StatusRejected, StatusWithdrawn:
		return true
	}
	return false
}

// IsEngaged reports whether the tenant has actually applied. Engaged
// applications are the ones follow-up SLAs act on.
func (s Status) IsEngaged() bool {
	switch s {
	case StatusApplied, StatusScreening, StatusInterview, StatusOffer:
		return true
	}
	return false
}

// allowedTransitions is the pipeline graph. A status maps to the set it may
// move to; the empty set means terminal.
//
// StatusInterview self-loops because multiple interview rounds are normal and
// each one is worth its own event. StatusGhosted can move back into the
// pipeline because employers do resurface after months of silence.
var allowedTransitions = map[Status]map[Status]bool{
	StatusDiscovered:  set(StatusScreenedOut, StatusQueued, StatusApplied, StatusWithdrawn),
	StatusScreenedOut: set(StatusQueued, StatusApplied),
	StatusQueued:      set(StatusApplied, StatusScreenedOut, StatusWithdrawn),
	StatusApplied:     set(StatusScreening, StatusInterview, StatusOffer, StatusRejected, StatusGhosted, StatusWithdrawn),
	StatusScreening:   set(StatusInterview, StatusOffer, StatusRejected, StatusGhosted, StatusWithdrawn),
	StatusInterview:   set(StatusInterview, StatusOffer, StatusRejected, StatusGhosted, StatusWithdrawn),
	StatusOffer:       set(StatusAccepted, StatusRejected, StatusWithdrawn),
	StatusGhosted:     set(StatusScreening, StatusInterview, StatusOffer, StatusRejected, StatusWithdrawn),
	StatusAccepted:    set(),
	StatusRejected:    set(),
	StatusWithdrawn:   set(),
}

func set(statuses ...Status) map[Status]bool {
	m := make(map[Status]bool, len(statuses))
	for _, s := range statuses {
		m[s] = true
	}
	return m
}

// CanTransitionTo reports whether s may move to next.
func (s Status) CanTransitionTo(next Status) bool {
	return allowedTransitions[s][next]
}

// ValidateTransition returns a descriptive error when the move is not allowed.
func ValidateTransition(from, to Status) error {
	if !from.Valid() {
		return NewValidationError(fmt.Sprintf("current status %q is not recognised", from))
	}
	if !to.Valid() {
		return NewValidationError(fmt.Sprintf("status %q is not recognised", to))
	}
	if from == to {
		// Re-asserting the same status is a no-op rather than an error, except
		// for interview rounds where repetition is meaningful.
		if from == StatusInterview {
			return nil
		}
		return NewValidationError(fmt.Sprintf("application is already %q", to))
	}
	if !from.CanTransitionTo(to) {
		allowed := make([]string, 0, len(allowedTransitions[from]))
		for s := range allowedTransitions[from] {
			allowed = append(allowed, string(s))
		}
		if len(allowed) == 0 {
			return NewValidationError(fmt.Sprintf("%q is terminal and cannot transition to %q", from, to))
		}
		return NewValidationError(fmt.Sprintf("cannot move from %q to %q; allowed: %v", from, to, allowed))
	}
	return nil
}

// Application is the tenant's relationship to a Posting, including how it was
// pursued and where it stands.
type Application struct {
	ID        ID
	TenantID  ID
	PostingID ID

	Status    Status
	AppliedAt *time.Time

	// ResumeVariant labels which resume was sent. Only the label, URL, and a
	// content hash are stored; ApplyTrail never holds the file itself, which
	// keeps a free-tier deployment free.
	ResumeVariant      string
	ResumeURL          string
	ResumeHash         string
	CoverLetterVariant string

	// NextActionAt drives the follow-up queue.
	NextActionAt *time.Time
	Notes        string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EventType classifies an entry in the application timeline.
type EventType string

const (
	EventCreated       EventType = "created"
	EventStatusChanged EventType = "status_changed"
	EventNoteAdded     EventType = "note_added"
	EventContactLogged EventType = "contact_logged"
	EventFollowUpSent  EventType = "follow_up_sent"
	EventRediscovered  EventType = "rediscovered"
)

// ApplicationEvent is one append-only entry in an application's timeline.
// Events are never updated or deleted: the timeline is the audit trail that
// lets an agent reason about how long a stage has been running.
type ApplicationEvent struct {
	ID            ID
	TenantID      ID
	ApplicationID ID

	Type       EventType
	FromStatus *Status
	ToStatus   *Status

	OccurredAt time.Time
	// Actor records who caused the event: an API key label, "system", or a
	// human name.
	Actor   string
	Payload []byte

	CreatedAt time.Time
}
