package httpapi

import (
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// The wire types live here rather than reusing domain structs directly, so the
// public API stays stable when the internal model changes. Field names are
// snake_case and stable; adding a field is safe, renaming one is not.

type candidateDTO struct {
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Company     string `json:"company,omitempty"`
	Location    string `json:"location,omitempty"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`

	PostedAt        *time.Time `json:"posted_at,omitempty"`
	RemoteType      string     `json:"remote_type,omitempty"`
	Seniority       string     `json:"seniority,omitempty"`
	CompMin         *int64     `json:"comp_min,omitempty"`
	CompMax         *int64     `json:"comp_max,omitempty"`
	CompCurrency    string     `json:"comp_currency,omitempty"`
	VisaSponsorship *bool      `json:"visa_sponsorship,omitempty"`
	TechTags        []string   `json:"tech_tags,omitempty"`
}

func (c candidateDTO) toDomain() domain.Candidate {
	return domain.Candidate{
		URL:         c.URL,
		Title:       c.Title,
		Company:     c.Company,
		Location:    c.Location,
		Source:      domain.Source(c.Source),
		Description: c.Description,
		PostedAt:    c.PostedAt,
		RemoteType:  domain.RemoteType(c.RemoteType),
		Seniority:   domain.Seniority(c.Seniority),
		Compensation: domain.Compensation{
			Min:      c.CompMin,
			Max:      c.CompMax,
			Currency: c.CompCurrency,
		},
		VisaSponsorship: c.VisaSponsorship,
		TechTags:        c.TechTags,
	}
}

func candidatesToDomain(dtos []candidateDTO) []domain.Candidate {
	candidates := make([]domain.Candidate, len(dtos))
	for i, dto := range dtos {
		candidates[i] = dto.toDomain()
	}
	return candidates
}

type filterRequestDTO struct {
	Profile         string         `json:"profile,omitempty"`
	ResurfacePolicy map[string]int `json:"resurface_policy,omitempty"`
	SearchRunID     string         `json:"search_run_id,omitempty"`
	Candidates      []candidateDTO `json:"candidates"`
}

type resultDTO struct {
	Index      int     `json:"index"`
	Verdict    string  `json:"verdict"`
	MatchTier  string  `json:"match_tier"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`

	PostingID         string     `json:"posting_id,omitempty"`
	ApplicationID     string     `json:"application_id,omitempty"`
	ApplicationStatus string     `json:"application_status,omitempty"`
	FirstSeenAt       *time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	DaysSince         *int       `json:"days_since,omitempty"`
}

func resultToDTO(r domain.FilterResult) resultDTO {
	dto := resultDTO{
		Index:       r.Index,
		Verdict:     string(r.Verdict),
		MatchTier:   string(r.Tier),
		Confidence:  r.Confidence,
		Reason:      r.Reason,
		FirstSeenAt: r.FirstSeenAt,
		LastSeenAt:  r.LastSeenAt,
		DaysSince:   r.DaysSince,
	}
	if r.PostingID != nil {
		dto.PostingID = r.PostingID.String()
	}
	if r.ApplicationID != nil {
		dto.ApplicationID = r.ApplicationID.String()
	}
	if r.ApplicationStatus != nil {
		dto.ApplicationStatus = string(*r.ApplicationStatus)
	}
	return dto
}

type filterResponseDTO struct {
	Results        []resultDTO     `json:"results"`
	Summary        service.Summary `json:"summary"`
	ProfileApplied string          `json:"profile_applied,omitempty"`
}

type ingestRequestDTO struct {
	Profile         string         `json:"profile,omitempty"`
	ResurfacePolicy map[string]int `json:"resurface_policy,omitempty"`
	SearchRunID     string         `json:"search_run_id,omitempty"`
	Candidates      []candidateDTO `json:"candidates"`
}

type ingestResultDTO struct {
	resultDTO
	Created bool `json:"created"`
}

type ingestResponseDTO struct {
	Results        []ingestResultDTO `json:"results"`
	Summary        service.Summary   `json:"summary"`
	ProfileApplied string            `json:"profile_applied,omitempty"`
}

type applicationDTO struct {
	ID        string `json:"id"`
	PostingID string `json:"posting_id"`
	Status    string `json:"status"`

	Company        string `json:"company,omitempty"`
	Title          string `json:"title,omitempty"`
	URL            string `json:"url,omitempty"`
	LocationBucket string `json:"location,omitempty"`

	AppliedAt          *time.Time `json:"applied_at,omitempty"`
	ResumeVariant      string     `json:"resume_variant,omitempty"`
	ResumeURL          string     `json:"resume_url,omitempty"`
	CoverLetterVariant string     `json:"cover_letter_variant,omitempty"`
	NextActionAt       *time.Time `json:"next_action_at,omitempty"`
	Notes              string     `json:"notes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func applicationToDTO(a domain.Application) applicationDTO {
	return applicationDTO{
		ID:                 a.ID.String(),
		PostingID:          a.PostingID.String(),
		Status:             string(a.Status),
		AppliedAt:          a.AppliedAt,
		ResumeVariant:      a.ResumeVariant,
		ResumeURL:          a.ResumeURL,
		CoverLetterVariant: a.CoverLetterVariant,
		NextActionAt:       a.NextActionAt,
		Notes:              a.Notes,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

func applicationWithPostingToDTO(a store.ApplicationWithPosting) applicationDTO {
	dto := applicationToDTO(a.Application)
	dto.Company = a.CompanyName
	dto.Title = a.PostingTitle
	dto.URL = a.PostingURL
	dto.LocationBucket = a.LocationBucket
	return dto
}

type eventDTO struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Actor      string    `json:"actor,omitempty"`
}

func eventToDTO(e domain.ApplicationEvent) eventDTO {
	dto := eventDTO{
		ID:         e.ID.String(),
		Type:       string(e.Type),
		OccurredAt: e.OccurredAt,
		Actor:      e.Actor,
	}
	if e.FromStatus != nil {
		dto.FromStatus = string(*e.FromStatus)
	}
	if e.ToStatus != nil {
		dto.ToStatus = string(*e.ToStatus)
	}
	return dto
}

type contactDTO struct {
	ID            string     `json:"id"`
	Name          string     `json:"name,omitempty"`
	Role          string     `json:"role,omitempty"`
	Email         string     `json:"email,omitempty"`
	LinkedInURL   string     `json:"linkedin_url,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	LastContactAt *time.Time `json:"last_contact_at,omitempty"`
}

func contactToDTO(c domain.Contact) contactDTO {
	return contactDTO{
		ID:            c.ID.String(),
		Name:          c.Name,
		Role:          c.Role,
		Email:         c.Email,
		LinkedInURL:   c.LinkedInURL,
		Notes:         c.Notes,
		LastContactAt: c.LastContactAt,
	}
}

type followUpDTO struct {
	applicationDTO
	DaysSinceApplied int `json:"days_since_applied"`
	DaysOverdue      int `json:"days_overdue"`
}

type profileDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Active      bool   `json:"active"`

	Keywords  []string `json:"keywords"`
	Titles    []string `json:"titles"`
	Locations []string `json:"locations"`

	RemoteOnly      bool     `json:"remote_only"`
	MinCompensation *int64   `json:"min_compensation,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	Seniority       []string `json:"seniority"`

	CompanyAllowlist []string `json:"company_allowlist"`
	CompanyBlocklist []string `json:"company_blocklist"`
	ExcludeKeywords  []string `json:"exclude_keywords"`

	ResurfacePolicy map[string]int `json:"resurface_policy"`
}

func profileToDTO(p domain.SearchProfile) profileDTO {
	seniority := make([]string, len(p.Seniority))
	for i, s := range p.Seniority {
		seniority[i] = string(s)
	}
	return profileDTO{
		ID:               p.ID.String(),
		Name:             p.Name,
		Description:      p.Description,
		Active:           p.Active,
		Keywords:         orEmpty(p.Keywords),
		Titles:           orEmpty(p.Titles),
		Locations:        orEmpty(p.Locations),
		RemoteOnly:       p.RemoteOnly,
		MinCompensation:  p.MinCompensation,
		Currency:         p.Currency,
		Seniority:        orEmpty(seniority),
		CompanyAllowlist: orEmpty(p.CompanyAllowlist),
		CompanyBlocklist: orEmpty(p.CompanyBlocklist),
		ExcludeKeywords:  orEmpty(p.ExcludeKeywords),
		ResurfacePolicy:  policyToMap(p.ResurfacePolicy.CooldownDays),
	}
}

func policyToMap(cooldowns map[domain.Status]int) map[string]int {
	out := make(map[string]int, len(cooldowns))
	for status, days := range cooldowns {
		out[string(status)] = days
	}
	return out
}

// policyFromMap converts a wire policy, validating status names so a typo
// surfaces as a 400 rather than being silently ignored at match time.
func policyFromMap(raw map[string]int) (*domain.ResurfacePolicy, error) {
	if raw == nil {
		return nil, nil
	}
	policy := domain.ResurfacePolicy{CooldownDays: make(map[domain.Status]int, len(raw))}
	for name, days := range raw {
		policy.CooldownDays[domain.Status(name)] = days
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &policy, nil
}

// orEmpty renders a nil slice as [] rather than null, so clients can iterate
// without a nil check.
func orEmpty[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
