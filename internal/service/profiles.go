package service

import (
	"context"
	"errors"
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

// ProfileView is a profile resolved for a caller about to run a search.
//
// It carries the merged resurfacing policy rather than the sparse override
// stored on the row: an agent reading a profile should see every cooldown that
// will actually apply, not just the ones this profile chose to change.
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

	// now is already UTC (Service.now, service.go), so this is a UTC calendar
	// day. That is a deliberate, documented choice, not a per-tenant
	// timezone: the schema has no notion of the operator's local time.
	now := s.now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	runsToday, err := s.store.SearchRuns().CountSince(ctx, p.TenantID, profile.ID, startOfDay)
	if err != nil {
		return ProfileView{}, err
	}
	lastRun, err := s.store.SearchRuns().List(ctx, p.TenantID, store.SearchRunFilter{
		ProfileID: &profile.ID,
		Limit:     1,
	})
	if err != nil {
		return ProfileView{}, err
	}

	view := ProfileView{
		Profile:                  profile,
		EffectiveResurfacePolicy: effectivePolicy(profile.ResurfacePolicy),
		RunsToday:                runsToday,
	}
	if len(lastRun) > 0 {
		view.LastRunAt = &lastRun[0].StartedAt
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
// StartRun requires only read scope, deliberately: a search_runs row is audit
// bookkeeping for the run, not a change to tracked postings or applications,
// and the get_search_profile MCP tool's start_run flag is advertised with
// ReadOnlyHint — a genuinely read-only search agent must be able to use it.
// FilterPostings's recordRunProgress makes the same call for the same reason.
func (s *Service) StartRun(ctx context.Context, p domain.Principal, profileName string, source domain.Source, query, notes string) (domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return domain.SearchRun{}, err
	}

	run := domain.SearchRun{TenantID: p.TenantID, Source: source, Query: query, Notes: notes}
	var profileNameToApply string
	if profileName != "" {
		profile, err := s.store.SearchProfiles().GetByName(ctx, p.TenantID, profileName)
		if err != nil {
			return domain.SearchRun{}, err
		}
		run.ProfileID = &profile.ID
		profileNameToApply = profile.Name
	}
	created, err := s.store.SearchRuns().Create(ctx, run)
	if err != nil {
		return domain.SearchRun{}, err
	}
	created.ProfileName = profileNameToApply
	return created, nil
}

// FinishRun closes a search run.
func (s *Service) FinishRun(ctx context.Context, p domain.Principal, id domain.ID, notes string) (domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeWrite); err != nil {
		return domain.SearchRun{}, err
	}
	run, err := s.store.SearchRuns().Finish(ctx, p.TenantID, id, s.now(), notes)
	if err != nil {
		return domain.SearchRun{}, err
	}
	return s.withProfileName(ctx, p.TenantID, run)
}

// ListRuns returns recent search runs.
func (s *Service) ListRuns(ctx context.Context, p domain.Principal, filter store.SearchRunFilter) ([]domain.SearchRun, error) {
	if err := p.RequireScope(domain.ScopeRead); err != nil {
		return nil, err
	}
	runs, err := s.store.SearchRuns().List(ctx, p.TenantID, filter)
	if err != nil {
		return nil, err
	}

	ids := make([]domain.ID, 0, len(runs))
	seen := make(map[domain.ID]bool, len(runs))
	for _, r := range runs {
		if r.ProfileID != nil && !seen[*r.ProfileID] {
			seen[*r.ProfileID] = true
			ids = append(ids, *r.ProfileID)
		}
	}
	if len(ids) == 0 {
		return runs, nil
	}
	names, err := s.store.SearchProfiles().NamesByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, err
	}
	for i, r := range runs {
		if r.ProfileID != nil {
			runs[i].ProfileName = names[*r.ProfileID]
		}
	}
	return runs, nil
}

// withProfileName resolves and fills in a single run's profile name. A
// deleted profile (ProfileID set but no longer resolvable) leaves the name
// blank rather than failing the request: the run itself is still valid.
func (s *Service) withProfileName(ctx context.Context, tenantID domain.ID, run domain.SearchRun) (domain.SearchRun, error) {
	if run.ProfileID == nil {
		return run, nil
	}
	profile, err := s.store.SearchProfiles().Get(ctx, tenantID, *run.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return run, nil
		}
		return domain.SearchRun{}, err
	}
	run.ProfileName = profile.Name
	return run, nil
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
