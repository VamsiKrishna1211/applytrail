package httpapi

import (
	"net/http"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
)

type upsertContactRequestDTO struct {
	Name          string     `json:"name,omitempty"`
	Role          string     `json:"role,omitempty"`
	Email         string     `json:"email,omitempty"`
	LinkedInURL   string     `json:"linkedin_url,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	LastContactAt *time.Time `json:"last_contact_at,omitempty"`

	Company       string `json:"company,omitempty"`
	ApplicationID string `json:"application_id,omitempty"`
}

func (h *handlers) handleUpsertContact(w http.ResponseWriter, r *http.Request) {
	var body upsertContactRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	applicationID, err := optionalID("application_id", body.ApplicationID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	contact, err := h.svc.UpsertContact(r.Context(), principalOf(r), service.UpsertContactRequest{
		Contact: domain.Contact{
			Name:          body.Name,
			Role:          body.Role,
			Email:         body.Email,
			LinkedInURL:   body.LinkedInURL,
			Notes:         body.Notes,
			LastContactAt: body.LastContactAt,
		},
		CompanyName:         body.Company,
		LinkToApplicationID: applicationID,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, contactToDTO(contact))
}

type listContactsResponseDTO struct {
	Contacts []contactDTO `json:"contacts"`
}

func (h *handlers) handleListContacts(w http.ResponseWriter, r *http.Request) {
	companyID, err := optionalID("company_id", r.URL.Query().Get("company_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	contacts, err := h.svc.ListContacts(r.Context(), principalOf(r), companyID, intParam(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listContactsResponseDTO{Contacts: make([]contactDTO, len(contacts))}
	for i, c := range contacts {
		resp.Contacts[i] = contactToDTO(c)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type statusCountDTO struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type resumeVariantDTO struct {
	Variant      string  `json:"variant"`
	Sent         int     `json:"sent"`
	Responded    int     `json:"responded"`
	Rejected     int     `json:"rejected"`
	Ghosted      int     `json:"ghosted"`
	ResponseRate float64 `json:"response_rate"`
}

type statsResponseDTO struct {
	ByStatus       []statusCountDTO   `json:"by_status"`
	ResumeVariants []resumeVariantDTO `json:"resume_variants"`
	TotalTracked   int                `json:"total_tracked"`
	TotalApplied   int                `json:"total_applied"`
	ResponseRate   float64            `json:"response_rate"`
}

func (h *handlers) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetStats(r.Context(), principalOf(r))
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := statsResponseDTO{
		ByStatus:       make([]statusCountDTO, len(stats.ByStatus)),
		ResumeVariants: make([]resumeVariantDTO, len(stats.ResumeVariants)),
		TotalTracked:   stats.TotalTracked,
		TotalApplied:   stats.TotalApplied,
		ResponseRate:   stats.ResponseRate,
	}
	for i, c := range stats.ByStatus {
		resp.ByStatus[i] = statusCountDTO{Status: string(c.Status), Count: c.Count}
	}
	for i, v := range stats.ResumeVariants {
		resp.ResumeVariants[i] = resumeVariantDTO{
			Variant:      v.Variant,
			Sent:         v.Sent,
			Responded:    v.Responded,
			Rejected:     v.Rejected,
			Ghosted:      v.Ghosted,
			ResponseRate: v.ResponseRate,
		}
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type apiKeyDTO struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Label      string     `json:"label,omitempty"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func apiKeyToDTO(k domain.APIKey) apiKeyDTO {
	scopes := make([]string, len(k.Scopes))
	for i, s := range k.Scopes {
		scopes[i] = string(s)
	}
	return apiKeyDTO{
		ID:         k.ID.String(),
		Prefix:     k.Prefix,
		Label:      k.Label,
		Scopes:     scopes,
		LastUsedAt: k.LastUsedAt,
		ExpiresAt:  k.ExpiresAt,
		RevokedAt:  k.RevokedAt,
		CreatedAt:  k.CreatedAt,
	}
}

type createKeyRequestDTO struct {
	Label     string     `json:"label,omitempty"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type createKeyResponseDTO struct {
	apiKeyDTO
	// Secret is returned exactly once, here. It cannot be recovered later,
	// because only its hash is stored.
	Secret string `json:"secret"`
}

func (h *handlers) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var body createKeyRequestDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	scopes, err := domain.ParseScopes(body.Scopes)
	if err != nil {
		writeError(w, r, err)
		return
	}

	issued, err := h.svc.CreateAPIKey(r.Context(), principalOf(r), body.Label, scopes, body.ExpiresAt)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, createKeyResponseDTO{
		apiKeyDTO: apiKeyToDTO(issued.Key),
		Secret:    issued.Secret,
	})
}

type listKeysResponseDTO struct {
	Keys []apiKeyDTO `json:"keys"`
}

func (h *handlers) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.svc.ListAPIKeys(r.Context(), principalOf(r))
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listKeysResponseDTO{Keys: make([]apiKeyDTO, len(keys))}
	for i, k := range keys {
		resp.Keys[i] = apiKeyToDTO(k)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func (h *handlers) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}

	key, err := h.svc.RevokeAPIKey(r.Context(), principalOf(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, apiKeyToDTO(key))
}
