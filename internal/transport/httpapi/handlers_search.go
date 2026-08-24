package httpapi

import (
	"net/http"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

type createProfileRequestDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Active      *bool  `json:"active,omitempty"`

	Keywords  []string `json:"keywords,omitempty"`
	Titles    []string `json:"titles,omitempty"`
	Locations []string `json:"locations,omitempty"`

	RemoteOnly      bool     `json:"remote_only,omitempty"`
	MinCompensation *int64   `json:"min_compensation,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	Seniority       []string `json:"seniority,omitempty"`

	CompanyAllowlist []string `json:"company_allowlist,omitempty"`
	CompanyBlocklist []string `json:"company_blocklist,omitempty"`
	ExcludeKeywords  []string `json:"exclude_keywords,omitempty"`

	ResurfacePolicy map[string]int `json:"resurface_policy,omitempty"`
}

func (h *handlers) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var body createProfileRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	policy, err := policyFromMap(body.ResurfacePolicy)
	if err != nil {
		writeError(w, r, err)
		return
	}

	profile := domain.SearchProfile{
		Name:             body.Name,
		Description:      body.Description,
		Active:           body.Active == nil || *body.Active, // new profiles are active unless told otherwise
		Keywords:         body.Keywords,
		Titles:           body.Titles,
		Locations:        body.Locations,
		RemoteOnly:       body.RemoteOnly,
		MinCompensation:  body.MinCompensation,
		Currency:         body.Currency,
		Seniority:        toSeniorities(body.Seniority),
		CompanyAllowlist: body.CompanyAllowlist,
		CompanyBlocklist: body.CompanyBlocklist,
		ExcludeKeywords:  body.ExcludeKeywords,
	}
	if policy != nil {
		profile.ResurfacePolicy = *policy
	}

	created, err := h.svc.CreateProfile(r.Context(), principalOf(r), profile)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, profileToDTO(created))
}

type profileViewDTO struct {
	Profile profileDTO `json:"profile"`
	// EffectiveResurfacePolicy is the profile's overrides merged onto the
	// defaults, so a caller sees every cooldown that will actually apply.
	EffectiveResurfacePolicy map[string]int `json:"effective_resurface_policy"`
	RunsToday                int            `json:"runs_today"`
	LastRunAt                *time.Time     `json:"last_run_at,omitempty"`
}

func (h *handlers) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.GetProfile(r.Context(), principalOf(r), chiParam(r, "name"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, profileViewDTO{
		Profile:                  profileToDTO(view.Profile),
		EffectiveResurfacePolicy: policyToMap(view.EffectiveResurfacePolicy),
		RunsToday:                view.RunsToday,
		LastRunAt:                view.LastRunAt,
	})
}

type listProfilesResponseDTO struct {
	Profiles []profileDTO `json:"profiles"`
}

func (h *handlers) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"

	profiles, err := h.svc.ListProfiles(r.Context(), principalOf(r), activeOnly)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listProfilesResponseDTO{Profiles: make([]profileDTO, len(profiles))}
	for i, p := range profiles {
		resp.Profiles[i] = profileToDTO(p)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type updateProfileRequestDTO struct {
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`

	Keywords  []string `json:"keywords,omitempty"`
	Titles    []string `json:"titles,omitempty"`
	Locations []string `json:"locations,omitempty"`

	RemoteOnly      *bool    `json:"remote_only,omitempty"`
	MinCompensation *int64   `json:"min_compensation,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	Seniority       []string `json:"seniority,omitempty"`

	CompanyAllowlist []string `json:"company_allowlist,omitempty"`
	CompanyBlocklist []string `json:"company_blocklist,omitempty"`
	ExcludeKeywords  []string `json:"exclude_keywords,omitempty"`

	ResurfacePolicy map[string]int `json:"resurface_policy,omitempty"`
}

func (h *handlers) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body updateProfileRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	policy, err := policyFromMap(body.ResurfacePolicy)
	if err != nil {
		writeError(w, r, err)
		return
	}

	updated, err := h.svc.UpdateProfile(r.Context(), principalOf(r), id, store.SearchProfileUpdate{
		Description:      body.Description,
		Active:           body.Active,
		Keywords:         body.Keywords,
		Titles:           body.Titles,
		Locations:        body.Locations,
		RemoteOnly:       body.RemoteOnly,
		MinCompensation:  body.MinCompensation,
		Currency:         body.Currency,
		Seniority:        toSeniorities(body.Seniority),
		CompanyAllowlist: body.CompanyAllowlist,
		CompanyBlocklist: body.CompanyBlocklist,
		ExcludeKeywords:  body.ExcludeKeywords,
		ResurfacePolicy:  policy,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, profileToDTO(updated))
}

type searchRunDTO struct {
	ID      string `json:"id"`
	Profile string `json:"profile,omitempty"`
	Source  string `json:"source,omitempty"`
	Query   string `json:"query,omitempty"`

	CandidatesSeen   int `json:"candidates_seen"`
	NewCount         int `json:"new_count"`
	DuplicateCount   int `json:"duplicate_count"`
	NeedsReviewCount int `json:"needs_review_count"`

	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Notes      string     `json:"notes,omitempty"`
}

func searchRunToDTO(run domain.SearchRun) searchRunDTO {
	return searchRunDTO{
		ID:               run.ID.String(),
		Source:           string(run.Source),
		Query:            run.Query,
		CandidatesSeen:   run.CandidatesSeen,
		NewCount:         run.NewCount,
		DuplicateCount:   run.DuplicateCount,
		NeedsReviewCount: run.NeedsReviewCount,
		StartedAt:        run.StartedAt,
		FinishedAt:       run.FinishedAt,
		Notes:            run.Notes,
	}
}

type startRunRequestDTO struct {
	Profile string `json:"profile,omitempty"`
	Source  string `json:"source,omitempty"`
	Query   string `json:"query,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

func (h *handlers) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body startRunRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	run, err := h.svc.StartRun(r.Context(), principalOf(r), body.Profile, domain.Source(body.Source), body.Query, body.Notes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, searchRunToDTO(run))
}

type finishRunRequestDTO struct {
	Notes string `json:"notes,omitempty"`
}

func (h *handlers) handleFinishRun(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body finishRunRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	run, err := h.svc.FinishRun(r.Context(), principalOf(r), id, body.Notes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, searchRunToDTO(run))
}

type listRunsResponseDTO struct {
	Runs []searchRunDTO `json:"runs"`
}

func (h *handlers) handleListRuns(w http.ResponseWriter, r *http.Request) {
	filter := store.SearchRunFilter{Limit: intParam(r.URL.Query().Get("limit"), 25)}
	if raw := r.URL.Query().Get("since"); raw != "" {
		since, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, r, domain.NewValidationError("since must be an RFC3339 timestamp"))
			return
		}
		filter.Since = &since
	}

	runs, err := h.svc.ListRuns(r.Context(), principalOf(r), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listRunsResponseDTO{Runs: make([]searchRunDTO, len(runs))}
	for i, run := range runs {
		resp.Runs[i] = searchRunToDTO(run)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func toSeniorities(raw []string) []domain.Seniority {
	if raw == nil {
		return nil
	}
	levels := make([]domain.Seniority, len(raw))
	for i, s := range raw {
		levels[i] = domain.Seniority(s)
	}
	return levels
}
