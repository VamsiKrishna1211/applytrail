package service

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// CreateProfile stores a new search profile.
func (s *Service) CreateProfile(ctx context.Context, p domain.Principal, profile domain.SearchProfile) (domain.SearchProfile, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.SearchProfile{}, err
	}
	profile.TenantID = p.TenantID
	if err := profile.Validate(); err != nil {
		return domain.SearchProfile{}, err
	}
	return s.store.SearchProfiles().Create(ctx, profile)
}

// GetProfile returns a profile by name, with its effective resurfacing rules
// resolved.
//
// Callers get the merged policy, not the sparse override stored on the row: an
// agent reading a profile should see every cooldown that will actually apply,
// not just the ones this profile chose to change.
type ProfileView struct {
	Profile domain.SearchProfile
	// EffectiveResurfacePolicy is the profile's overrides merged onto the
	// defaults.
	EffectiveResurfacePolicy map[domain.Status]int
	// RunsToday reports how many searches already ran against this profile,
	// so a routine can tell whether it has work left to do.
	RunsToday int
	LastRunAt *time.Time
}

// GetProfile resolves a profile for a caller about to run a search.
func (s *Service) GetProfile(ctx context.Context, p domain.Principal, name string) (ProfileView, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return ProfileView{}, err
	}

	profile, err := s.store.SearchProfiles().GetByName(ctx, p.TenantID, name)
	if err != nil {
		return ProfileView{}, err
	}

	now := s.now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	runs, err := s.store.SearchRuns().List(ctx, p.TenantID, store.SearchRunFilter{
		ProfileID: &profile.ID,
		Since:     &startOfDay,
		Limit:     100,
	})
	if err != nil {
		return ProfileView{}, err
	}

	view := ProfileView{
		Profile:                  profile,
		EffectiveResurfacePolicy: effectivePolicy(profile.ResurfacePolicy),
		RunsToday:                len(runs),
	}
	if len(runs) > 0 {
		view.LastRunAt = &runs[0].StartedAt
	}
	return view, nil
}

// ListProfiles returns the tenant's search profiles.
func (s *Service) ListProfiles(ctx context.Context, p domain.Principal, activeOnly bool) ([]domain.SearchProfile, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}
	return s.store.SearchProfiles().List(ctx, p.TenantID, activeOnly)
}

// UpdateProfile applies a partial update to a profile.
func (s *Service) UpdateProfile(ctx context.Context, p domain.Principal, id domain.ID, update store.SearchProfileUpdate) (domain.SearchProfile, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.SearchProfile{}, err
	}
	if update.ResurfacePolicy != nil {
		if err := update.ResurfacePolicy.Validate(); err != nil {
			return domain.SearchProfile{}, err
		}
	}
	for _, level := range update.Seniority {
		if !level.Valid() {
			return domain.SearchProfile{}, domain.NewValidationError("seniority " + string(level) + " is not a recognised level")
		}
	}
	return s.store.SearchProfiles().Update(ctx, p.TenantID, id, update)
}

// StartRun opens a search run for auditing.
func (s *Service) StartRun(ctx context.Context, p domain.Principal, profileName string, source domain.Source, query, notes string) (domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.SearchRun{}, err
	}

	run := domain.SearchRun{TenantID: p.TenantID, Source: source, Query: query, Notes: notes}
	if profileName != "" {
		profile, err := s.store.SearchProfiles().GetByName(ctx, p.TenantID, profileName)
		if err != nil {
			return domain.SearchRun{}, err
		}
		run.ProfileID = &profile.ID
	}
	return s.store.SearchRuns().Create(ctx, run)
}

// FinishRun closes a search run.
func (s *Service) FinishRun(ctx context.Context, p domain.Principal, id domain.ID, notes string) (domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.SearchRun{}, err
	}
	return s.store.SearchRuns().Finish(ctx, p.TenantID, id, s.now(), notes)
}

// ListRuns returns recent search runs.
func (s *Service) ListRuns(ctx context.Context, p domain.Principal, filter store.SearchRunFilter) ([]domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}
	return s.store.SearchRuns().List(ctx, p.TenantID, filter)
}

// effectivePolicy merges a profile's overrides onto the defaults.
func effectivePolicy(override domain.ResurfacePolicy) map[domain.Status]int {
	merged := map[domain.Status]int{}
	for status, days := range domain.DefaultResurfacePolicy().CooldownDays {
		merged[status] = days
	}
	for status, days := range override.CooldownDays {
		merged[status] = days
	}
	return merged
}
