package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

type searchProfileStore struct{ q *sqlcgen.Queries }

func (s searchProfileStore) Create(ctx context.Context, p domain.SearchProfile) (domain.SearchProfile, error) {
	policy, err := encodeResurfacePolicy(p.ResurfacePolicy)
	if err != nil {
		return domain.SearchProfile{}, err
	}

	row, err := s.q.CreateSearchProfile(ctx, sqlcgen.CreateSearchProfileParams{
		TenantID:         p.TenantID,
		Name:             p.Name,
		Description:      p.Description,
		Active:           p.Active,
		Keywords:         orEmptySlice(p.Keywords),
		Titles:           orEmptySlice(p.Titles),
		Locations:        orEmptySlice(p.Locations),
		RemoteOnly:       p.RemoteOnly,
		MinCompensation:  p.MinCompensation,
		Currency:         p.Currency,
		Seniority:        orEmptySlice(fromSeniorities(p.Seniority)),
		CompanyAllowlist: orEmptySlice(p.CompanyAllowlist),
		CompanyBlocklist: orEmptySlice(p.CompanyBlocklist),
		ExcludeKeywords:  orEmptySlice(p.ExcludeKeywords),
		ResurfacePolicy:  policy,
	})
	if err != nil {
		return domain.SearchProfile{}, translate("create search profile", "search profile", err)
	}
	return toDomainSearchProfile(row)
}

func (s searchProfileStore) Get(ctx context.Context, tenantID, id domain.ID) (domain.SearchProfile, error) {
	row, err := s.q.GetSearchProfile(ctx, sqlcgen.GetSearchProfileParams{ID: id, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SearchProfile{}, notFound("search profile", id)
	}
	if err != nil {
		return domain.SearchProfile{}, translate("get search profile", "search profile", err)
	}
	return toDomainSearchProfile(row)
}

func (s searchProfileStore) GetByName(ctx context.Context, tenantID domain.ID, name string) (domain.SearchProfile, error) {
	row, err := s.q.GetSearchProfileByName(ctx, sqlcgen.GetSearchProfileByNameParams{
		TenantID: tenantID,
		Name:     name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SearchProfile{}, domain.NewNotFoundError("search profile", name)
	}
	if err != nil {
		return domain.SearchProfile{}, translate("get search profile by name", "search profile", err)
	}
	return toDomainSearchProfile(row)
}

func (s searchProfileStore) List(ctx context.Context, tenantID domain.ID, activeOnly bool) ([]domain.SearchProfile, error) {
	rows, err := s.q.ListSearchProfiles(ctx, sqlcgen.ListSearchProfilesParams{
		TenantID:   tenantID,
		ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, translate("list search profiles", "search profile", err)
	}
	profiles := make([]domain.SearchProfile, len(rows))
	for i, row := range rows {
		profile, err := toDomainSearchProfile(row)
		if err != nil {
			return nil, err
		}
		profiles[i] = profile
	}
	return profiles, nil
}

func (s searchProfileStore) Update(ctx context.Context, tenantID, id domain.ID, u store.SearchProfileUpdate) (domain.SearchProfile, error) {
	params := sqlcgen.UpdateSearchProfileParams{
		ID:               id,
		TenantID:         tenantID,
		Description:      u.Description,
		Active:           u.Active,
		Keywords:         u.Keywords,
		Titles:           u.Titles,
		Locations:        u.Locations,
		RemoteOnly:       u.RemoteOnly,
		MinCompensation:  u.MinCompensation,
		Currency:         u.Currency,
		CompanyAllowlist: u.CompanyAllowlist,
		CompanyBlocklist: u.CompanyBlocklist,
		ExcludeKeywords:  u.ExcludeKeywords,
	}
	if u.Seniority != nil {
		params.Seniority = fromSeniorities(u.Seniority)
	}
	if u.ResurfacePolicy != nil {
		policy, err := encodeResurfacePolicy(*u.ResurfacePolicy)
		if err != nil {
			return domain.SearchProfile{}, err
		}
		params.ResurfacePolicy = policy
	}

	row, err := s.q.UpdateSearchProfile(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SearchProfile{}, notFound("search profile", id)
	}
	if err != nil {
		return domain.SearchProfile{}, translate("update search profile", "search profile", err)
	}
	return toDomainSearchProfile(row)
}

func (s searchProfileStore) NamesByIDs(ctx context.Context, tenantID domain.ID, ids []domain.ID) (map[domain.ID]string, error) {
	if len(ids) == 0 {
		return map[domain.ID]string{}, nil
	}
	rows, err := s.q.GetSearchProfileNamesByIDs(ctx, sqlcgen.GetSearchProfileNamesByIDsParams{
		TenantID: tenantID,
		Ids:      ids,
	})
	if err != nil {
		return nil, translate("get search profile names", "search profile", err)
	}
	names := make(map[domain.ID]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

type searchRunStore struct{ q *sqlcgen.Queries }

func (s searchRunStore) Create(ctx context.Context, run domain.SearchRun) (domain.SearchRun, error) {
	row, err := s.q.CreateSearchRun(ctx, sqlcgen.CreateSearchRunParams{
		TenantID:  run.TenantID,
		ProfileID: run.ProfileID,
		Source:    string(run.Source),
		Query:     run.Query,
		Notes:     run.Notes,
	})
	if err != nil {
		return domain.SearchRun{}, translate("create search run", "search run", err)
	}
	return toDomainSearchRun(row), nil
}

func (s searchRunStore) RecordProgress(ctx context.Context, tenantID, id domain.ID, p store.SearchRunProgress) (domain.SearchRun, error) {
	row, err := s.q.RecordSearchRunProgress(ctx, sqlcgen.RecordSearchRunProgressParams{
		ID:                 id,
		TenantID:           tenantID,
		CandidatesSeen:     int32(p.CandidatesSeen),     //nolint:gosec // batch sizes are bounded by request limits
		NewCount:           int32(p.NewCount),           //nolint:gosec // ditto
		EligibleAgainCount: int32(p.EligibleAgainCount), //nolint:gosec // ditto
		DuplicateCount:     int32(p.DuplicateCount),     //nolint:gosec // ditto
		NeedsReviewCount:   int32(p.NeedsReviewCount),   //nolint:gosec // ditto
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SearchRun{}, notFound("search run", id)
	}
	if err != nil {
		return domain.SearchRun{}, translate("record search run progress", "search run", err)
	}
	return toDomainSearchRun(row), nil
}

func (s searchRunStore) Finish(ctx context.Context, tenantID, id domain.ID, finishedAt time.Time, notes string) (domain.SearchRun, error) {
	row, err := s.q.FinishSearchRun(ctx, sqlcgen.FinishSearchRunParams{
		ID:         id,
		TenantID:   tenantID,
		FinishedAt: &finishedAt,
		Notes:      notes,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SearchRun{}, notFound("search run", id)
	}
	if err != nil {
		return domain.SearchRun{}, translate("finish search run", "search run", err)
	}
	return toDomainSearchRun(row), nil
}

func (s searchRunStore) CountSince(ctx context.Context, tenantID, profileID domain.ID, since time.Time) (int, error) {
	count, err := s.q.CountSearchRunsSince(ctx, sqlcgen.CountSearchRunsSinceParams{
		TenantID:  tenantID,
		ProfileID: &profileID,
		Since:     since,
	})
	if err != nil {
		return 0, translate("count search runs", "search run", err)
	}
	return int(count), nil
}

func (s searchRunStore) List(ctx context.Context, tenantID domain.ID, f store.SearchRunFilter) ([]domain.SearchRun, error) {
	rows, err := s.q.ListSearchRuns(ctx, sqlcgen.ListSearchRunsParams{
		TenantID:  tenantID,
		ProfileID: f.ProfileID,
		Since:     f.Since,
		RowLimit:  clampLimit(f.Limit),
	})
	if err != nil {
		return nil, translate("list search runs", "search run", err)
	}
	runs := make([]domain.SearchRun, len(rows))
	for i, row := range rows {
		runs[i] = toDomainSearchRun(row)
	}
	return runs, nil
}
