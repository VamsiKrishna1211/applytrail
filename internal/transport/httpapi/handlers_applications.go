package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

type recordApplicationRequestDTO struct {
	PostingID string        `json:"posting_id,omitempty"`
	Posting   *candidateDTO `json:"posting,omitempty"`

	AppliedAt          *time.Time `json:"applied_at,omitempty"`
	ResumeVariant      string     `json:"resume_variant,omitempty"`
	ResumeURL          string     `json:"resume_url,omitempty"`
	ResumeHash         string     `json:"resume_hash,omitempty"`
	CoverLetterVariant string     `json:"cover_letter_variant,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	FollowUpInDays     int        `json:"follow_up_in_days,omitempty"`
}

func (h *handlers) handleRecordApplication(w http.ResponseWriter, r *http.Request) {
	var body recordApplicationRequestDTO
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	req := service.RecordApplicationRequest{
		AppliedAt:          body.AppliedAt,
		ResumeVariant:      body.ResumeVariant,
		ResumeURL:          body.ResumeURL,
		ResumeHash:         body.ResumeHash,
		CoverLetterVariant: body.CoverLetterVariant,
		Notes:              body.Notes,
		FollowUpInDays:     body.FollowUpInDays,
	}

	postingID, err := optionalID("posting_id", body.PostingID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	req.PostingID = postingID
	if body.Posting != nil {
		candidate := body.Posting.toDomain()
		req.Candidate = &candidate
	}

	application, err := h.svc.RecordApplication(r.Context(), principalOf(r), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, applicationToDTO(application))
}

type updateApplicationRequestDTO struct {
	Status          string     `json:"status,omitempty"`
	OccurredAt      *time.Time `json:"occurred_at,omitempty"`
	Note            string     `json:"note,omitempty"`
	NextActionAt    *time.Time `json:"next_action_at,omitempty"`
	ClearNextAction bool       `json:"clear_next_action,omitempty"`
}

func (h *handlers) handleUpdateApplication(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body updateApplicationRequestDTO
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if body.Status == "" {
		writeError(w, r, domain.NewValidationError("status is required"))
		return
	}

	application, err := h.svc.UpdateStatus(r.Context(), principalOf(r), service.UpdateStatusRequest{
		ApplicationID:   id,
		Status:          domain.Status(body.Status),
		OccurredAt:      body.OccurredAt,
		Note:            body.Note,
		NextActionAt:    body.NextActionAt,
		ClearNextAction: body.ClearNextAction,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, applicationToDTO(application))
}

type noteRequestDTO struct {
	Note string `json:"note"`
}

func (h *handlers) handleAddNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body noteRequestDTO
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	application, err := h.svc.AddNote(r.Context(), principalOf(r), id, body.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, applicationToDTO(application))
}

type listApplicationsResponseDTO struct {
	Applications []applicationDTO `json:"applications"`
	// NextCursor is empty when the page is the last one.
	NextCursor string `json:"next_cursor,omitempty"`
}

func (h *handlers) handleListApplications(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	limit, err := intParam("limit", query.Get("limit"), 50, maxLimit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	filter := store.ApplicationListFilter{Limit: limit}
	for _, status := range query["status"] {
		filter.Statuses = append(filter.Statuses, domain.Status(status))
	}
	filter.CompanyNormalizedName = query.Get("company")

	if raw := query.Get("since"); raw != "" {
		since, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, r, domain.NewValidationError("since must be an RFC3339 timestamp"))
			return
		}
		filter.Since = &since
	}
	if raw := query.Get("cursor"); raw != "" {
		cursor, err := decodeCursor(raw)
		if err != nil {
			writeError(w, r, err)
			return
		}
		filter.Cursor = cursor
	}

	rows, err := h.svc.ListApplications(r.Context(), principalOf(r), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listApplicationsResponseDTO{Applications: make([]applicationDTO, len(rows))}
	for i, row := range rows {
		resp.Applications[i] = applicationWithPostingToDTO(row)
	}
	// A full page implies there may be more; an empty or short page is the end.
	if len(rows) == filter.Limit && filter.Limit > 0 {
		last := rows[len(rows)-1].Application
		resp.NextCursor = encodeCursor(store.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type applicationDetailDTO struct {
	Application applicationDTO `json:"application"`
	Events      []eventDTO     `json:"events"`
	Contacts    []contactDTO   `json:"contacts"`
}

func (h *handlers) handleGetApplication(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	eventLimit, err := intParam("event_limit", r.URL.Query().Get("event_limit"), 100, maxLimit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	detail, err := h.svc.GetApplication(r.Context(), principalOf(r), id, eventLimit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := applicationDetailDTO{
		Application: applicationWithPostingToDTO(detail.Application),
		Events:      make([]eventDTO, len(detail.Events)),
		Contacts:    make([]contactDTO, len(detail.Contacts)),
	}
	for i, e := range detail.Events {
		resp.Events[i] = eventToDTO(e)
	}
	for i, c := range detail.Contacts {
		resp.Contacts[i] = contactToDTO(c)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type followupsResponseDTO struct {
	Followups []followUpDTO `json:"followups"`
}

func (h *handlers) handleFollowups(w http.ResponseWriter, r *http.Request) {
	var asOf *time.Time
	if raw := r.URL.Query().Get("due_before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, r, domain.NewValidationError("due_before must be an RFC3339 timestamp"))
			return
		}
		asOf = &parsed
	}

	limit, err := intParam("limit", r.URL.Query().Get("limit"), 50, maxLimit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	followUps, err := h.svc.GetFollowupsDue(r.Context(), principalOf(r), asOf, limit)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := followupsResponseDTO{Followups: make([]followUpDTO, len(followUps))}
	for i, f := range followUps {
		resp.Followups[i] = followUpDTO{
			applicationDTO:   applicationWithPostingToDTO(f.ApplicationWithPosting),
			DaysSinceApplied: f.DaysSinceApplied,
			DaysOverdue:      f.DaysOverdue,
		}
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func pathID(r *http.Request, param string) (domain.ID, error) {
	return domain.ParseID(param, chi.URLParam(r, param))
}

// chiParam reads a path parameter that is not an identifier.
func chiParam(r *http.Request, param string) string {
	return chi.URLParam(r, param)
}

// maxLimit mirrors the store's own clampLimit ceiling (internal/store/postgres,
// shared by every list query). Keeping the handler's clamp equal to the
// store's is what makes intParam's returned value the value actually used,
// rather than a guess that can drift from it.
const maxLimit = 500

// intParam is for a limit-shaped query parameter: empty means "use the
// default", but anything else that is not a positive integer is the caller's
// mistake and must not be silently treated the same as if nothing had been
// sent — an agent that typo'd ?limit=fifty deserves a 400, not results it
// cannot tell were capped at the fallback instead of what it asked for.
// Anything above max is clamped rather than rejected, and the caller gets the
// clamped value back so pagination can be computed against what was actually
// used, not what was asked for.
func intParam(name, raw string, fallback, max int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, domain.NewValidationError(name + " must be a positive integer")
	}
	if n > max {
		return max, nil
	}
	return n, nil
}

// boolParam parses a boolean query parameter with strconv.ParseBool, which
// accepts 1/t/T/TRUE/true/True and the 0/f/F/FALSE/false/False equivalents.
// An unrecognised value ("yes", "TRU", a stray space) used to silently mean
// "false" no matter what it actually said; now it is a 400.
func boolParam(name, raw string, fallback bool) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, domain.NewValidationError(name + " must be a boolean")
	}
	return v, nil
}
