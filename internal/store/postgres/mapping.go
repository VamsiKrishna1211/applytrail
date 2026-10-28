package postgres

import (
	"encoding/json"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

// This file holds the only knowledge of how generated row structs line up with
// domain entities. Keeping it in one place means a schema change shows up as a
// compile error here rather than scattered across query methods.

func toDomainTenant(r sqlcgen.Tenant) domain.Tenant {
	return domain.Tenant{
		ID:        r.ID,
		Name:      r.Name,
		Slug:      r.Slug,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func toDomainAPIKey(r sqlcgen.ApiKey) domain.APIKey {
	return domain.APIKey{
		ID:         r.ID,
		TenantID:   r.TenantID,
		Prefix:     r.Prefix,
		Hash:       r.Hash,
		Label:      r.Label,
		Scopes:     toScopes(r.Scopes),
		LastUsedAt: r.LastUsedAt,
		ExpiresAt:  r.ExpiresAt,
		RevokedAt:  r.RevokedAt,
		CreatedAt:  r.CreatedAt,
	}
}

func toDomainCompany(r sqlcgen.Company) domain.Company {
	return domain.Company{
		ID:             r.ID,
		TenantID:       r.TenantID,
		Name:           r.Name,
		NormalizedName: r.NormalizedName,
		Domain:         r.Domain,
		CareersURL:     r.CareersUrl,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func toDomainPosting(r sqlcgen.Posting) domain.Posting {
	return domain.Posting{
		ID:              r.ID,
		TenantID:        r.TenantID,
		CompanyID:       r.CompanyID,
		Source:          domain.Source(r.Source),
		ExternalID:      r.ExternalID,
		CanonicalURL:    r.CanonicalUrl,
		RawURL:          r.RawUrl,
		Title:           r.Title,
		NormalizedTitle: r.NormalizedTitle,
		LocationRaw:     r.LocationRaw,
		LocationBucket:  r.LocationBucket,
		RemoteType:      domain.RemoteType(r.RemoteType),
		Seniority:       domain.Seniority(r.Seniority),
		Compensation: domain.Compensation{
			Min:      r.CompMin,
			Max:      r.CompMax,
			Currency: r.CompCurrency,
		},
		VisaSponsorship: r.VisaSponsorship,
		TechTags:        r.TechTags,
		DescriptionHash: r.DescriptionHash,
		Fingerprint:     r.Fingerprint,
		PostedAt:        r.PostedAt,
		FirstSeenAt:     r.FirstSeenAt,
		LastSeenAt:      r.LastSeenAt,
		ClosedAt:        r.ClosedAt,
		Raw:             r.Raw,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

func toDomainApplication(r sqlcgen.Application) domain.Application {
	return domain.Application{
		ID:                 r.ID,
		TenantID:           r.TenantID,
		PostingID:          r.PostingID,
		Status:             domain.Status(r.Status),
		AppliedAt:          r.AppliedAt,
		ResumeVariant:      r.ResumeVariant,
		ResumeURL:          r.ResumeUrl,
		ResumeHash:         r.ResumeHash,
		CoverLetterVariant: r.CoverLetterVariant,
		NextActionAt:       r.NextActionAt,
		Notes:              r.Notes,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

func toDomainEvent(r sqlcgen.ApplicationEvent) domain.ApplicationEvent {
	return domain.ApplicationEvent{
		ID:            r.ID,
		TenantID:      r.TenantID,
		ApplicationID: r.ApplicationID,
		Type:          domain.EventType(r.Type),
		FromStatus:    toStatusPtr(r.FromStatus),
		ToStatus:      toStatusPtr(r.ToStatus),
		OccurredAt:    r.OccurredAt,
		Actor:         r.Actor,
		Payload:       r.Payload,
		CreatedAt:     r.CreatedAt,
	}
}

func toDomainContact(r sqlcgen.Contact) domain.Contact {
	return domain.Contact{
		ID:            r.ID,
		TenantID:      r.TenantID,
		CompanyID:     r.CompanyID,
		Name:          r.Name,
		Role:          r.Role,
		Email:         r.Email,
		LinkedInURL:   r.LinkedinUrl,
		Notes:         r.Notes,
		LastContactAt: r.LastContactAt,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func toDomainSearchProfile(r sqlcgen.SearchProfile) (domain.SearchProfile, error) {
	policy, err := decodeResurfacePolicy(r.ResurfacePolicy)
	if err != nil {
		return domain.SearchProfile{}, err
	}
	return domain.SearchProfile{
		ID:               r.ID,
		TenantID:         r.TenantID,
		Name:             r.Name,
		Description:      r.Description,
		Active:           r.Active,
		Keywords:         r.Keywords,
		Titles:           r.Titles,
		Locations:        r.Locations,
		RemoteOnly:       r.RemoteOnly,
		MinCompensation:  r.MinCompensation,
		Currency:         r.Currency,
		Seniority:        toSeniorities(r.Seniority),
		CompanyAllowlist: r.CompanyAllowlist,
		CompanyBlocklist: r.CompanyBlocklist,
		ExcludeKeywords:  r.ExcludeKeywords,
		ResurfacePolicy:  policy,
	}, nil
}

func toDomainSearchRun(r sqlcgen.SearchRun) domain.SearchRun {
	return domain.SearchRun{
		ID:                 r.ID,
		TenantID:           r.TenantID,
		ProfileID:          r.ProfileID,
		Source:             domain.Source(r.Source),
		Query:              r.Query,
		CandidatesSeen:     int(r.CandidatesSeen),
		NewCount:           int(r.NewCount),
		EligibleAgainCount: int(r.EligibleAgainCount),
		DuplicateCount:     int(r.DuplicateCount),
		NeedsReviewCount:   int(r.NeedsReviewCount),
		StartedAt:          r.StartedAt,
		FinishedAt:         r.FinishedAt,
		Notes:              r.Notes,
	}
}

// decodeResurfacePolicy reads the per-status cooldown map. A profile records
// only its deviations, so an empty object is valid and means "use the
// defaults".
func decodeResurfacePolicy(raw json.RawMessage) (domain.ResurfacePolicy, error) {
	policy := domain.ResurfacePolicy{CooldownDays: map[domain.Status]int{}}
	if len(raw) == 0 {
		return policy, nil
	}
	var days map[string]int
	if err := json.Unmarshal(raw, &days); err != nil {
		return domain.ResurfacePolicy{}, domain.NewValidationError("resurface_policy is not a status-to-days object: " + err.Error())
	}
	for status, d := range days {
		policy.CooldownDays[domain.Status(status)] = d
	}
	return policy, nil
}

func encodeResurfacePolicy(policy domain.ResurfacePolicy) (json.RawMessage, error) {
	days := make(map[string]int, len(policy.CooldownDays))
	for status, d := range policy.CooldownDays {
		days[string(status)] = d
	}
	return json.Marshal(days)
}

func toScopes(raw []string) []domain.Scope {
	scopes := make([]domain.Scope, len(raw))
	for i, s := range raw {
		scopes[i] = domain.Scope(s)
	}
	return scopes
}

func fromScopes(scopes []domain.Scope) []string {
	raw := make([]string, len(scopes))
	for i, s := range scopes {
		raw[i] = string(s)
	}
	return raw
}

func toSeniorities(raw []string) []domain.Seniority {
	levels := make([]domain.Seniority, len(raw))
	for i, s := range raw {
		levels[i] = domain.Seniority(s)
	}
	return levels
}

func fromSeniorities(levels []domain.Seniority) []string {
	raw := make([]string, len(levels))
	for i, s := range levels {
		raw[i] = string(s)
	}
	return raw
}

func fromStatuses(statuses []domain.Status) []string {
	raw := make([]string, len(statuses))
	for i, s := range statuses {
		raw[i] = string(s)
	}
	return raw
}

func toStatusPtr(s *string) *domain.Status {
	if s == nil {
		return nil
	}
	status := domain.Status(*s)
	return &status
}

func fromStatusPtr(s *domain.Status) *string {
	if s == nil {
		return nil
	}
	raw := string(*s)
	return &raw
}

// orEmptySlice normalises a nil slice to an empty one. Every text[] column is
// NOT NULL with a '{}' default, and pgx sends a nil Go slice as SQL NULL, so
// omitting this turns "no tags" into a constraint violation.
func orEmptySlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// orEmptyJSON is the default for jsonb columns; the schema rejects NULL there.
func orEmptyJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
