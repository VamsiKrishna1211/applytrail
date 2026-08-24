package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

type applicationStore struct{ q *sqlcgen.Queries }

// defaultListLimit bounds an unbounded listing request.
const defaultListLimit = 50

// maxListLimit caps what a caller may ask for in one page.
const maxListLimit = 500

func clampLimit(limit int) int32 {
	switch {
	case limit <= 0:
		return defaultListLimit
	case limit > maxListLimit:
		return maxListLimit
	default:
		return int32(limit) //nolint:gosec // bounded by maxListLimit above
	}
}

// Insert relies on ON CONFLICT DO NOTHING, which returns no row when an
// application already exists for the posting. That is the idempotent case, so
// it is resolved by reading the existing row rather than reported as an error.
func (s applicationStore) Insert(ctx context.Context, app domain.Application) (domain.Application, error) {
	row, err := s.q.InsertApplication(ctx, sqlcgen.InsertApplicationParams{
		TenantID:           app.TenantID,
		PostingID:          app.PostingID,
		Status:             string(app.Status),
		AppliedAt:          app.AppliedAt,
		ResumeVariant:      app.ResumeVariant,
		ResumeUrl:          app.ResumeURL,
		ResumeHash:         app.ResumeHash,
		CoverLetterVariant: app.CoverLetterVariant,
		NextActionAt:       app.NextActionAt,
		Notes:              app.Notes,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetByPosting(ctx, app.TenantID, app.PostingID)
	}
	if err != nil {
		return domain.Application{}, translate("insert application", "application", err)
	}
	return toDomainApplication(row), nil
}

func (s applicationStore) Get(ctx context.Context, tenantID, id domain.ID) (domain.Application, error) {
	row, err := s.q.GetApplication(ctx, sqlcgen.GetApplicationParams{ID: id, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Application{}, notFound("application", id)
	}
	if err != nil {
		return domain.Application{}, translate("get application", "application", err)
	}
	return toDomainApplication(row), nil
}

func (s applicationStore) GetByPosting(ctx context.Context, tenantID, postingID domain.ID) (domain.Application, error) {
	row, err := s.q.GetApplicationByPosting(ctx, sqlcgen.GetApplicationByPostingParams{
		PostingID: postingID,
		TenantID:  tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Application{}, notFound("application for posting", postingID)
	}
	if err != nil {
		return domain.Application{}, translate("get application by posting", "application", err)
	}
	return toDomainApplication(row), nil
}

// ApplyTransition is a compare-and-swap on status. No rows updated means
// either the application is gone or someone else moved it first; the two are
// distinguished with a follow-up read so the caller gets an accurate error.
func (s applicationStore) ApplyTransition(ctx context.Context, tenantID, id domain.ID, t store.StatusTransition) (domain.Application, error) {
	row, err := s.q.UpdateApplicationStatus(ctx, sqlcgen.UpdateApplicationStatusParams{
		ID:              id,
		TenantID:        tenantID,
		FromStatus:      string(t.From),
		ToStatus:        string(t.To),
		AppliedAt:       t.AppliedAt,
		NextActionAt:    t.NextActionAt,
		ClearNextAction: t.ClearNextAction,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := s.Get(ctx, tenantID, id)
		if getErr != nil {
			return domain.Application{}, getErr
		}
		return domain.Application{}, fmt.Errorf(
			"%w: expected application to be %q but it is %q; another change landed first",
			domain.ErrConflict, t.From, current.Status)
	}
	if err != nil {
		return domain.Application{}, translate("update application status", "application", err)
	}
	return toDomainApplication(row), nil
}

func (s applicationStore) Update(ctx context.Context, tenantID, id domain.ID, u store.ApplicationUpdate) (domain.Application, error) {
	row, err := s.q.UpdateApplicationFields(ctx, sqlcgen.UpdateApplicationFieldsParams{
		ID:                 id,
		TenantID:           tenantID,
		ResumeVariant:      u.ResumeVariant,
		ResumeUrl:          u.ResumeURL,
		ResumeHash:         u.ResumeHash,
		CoverLetterVariant: u.CoverLetterVariant,
		NextActionAt:       u.NextActionAt,
		ClearNextAction:    u.ClearNextAction,
		Notes:              u.Notes,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Application{}, notFound("application", id)
	}
	if err != nil {
		return domain.Application{}, translate("update application", "application", err)
	}
	return toDomainApplication(row), nil
}

func (s applicationStore) List(ctx context.Context, tenantID domain.ID, f store.ApplicationListFilter) ([]store.ApplicationWithPosting, error) {
	params := sqlcgen.ListApplicationsParams{
		TenantID: tenantID,
		Since:    f.Since,
		RowLimit: clampLimit(f.Limit),
	}
	if len(f.Statuses) > 0 {
		params.Statuses = fromStatuses(f.Statuses)
	}
	if f.CompanyNormalizedName != "" {
		params.CompanyNormalizedName = &f.CompanyNormalizedName
	}
	if f.Cursor != nil {
		params.CursorCreatedAt = &f.Cursor.CreatedAt
		params.CursorID = &f.Cursor.ID
	}

	rows, err := s.q.ListApplications(ctx, params)
	if err != nil {
		return nil, translate("list applications", "application", err)
	}

	results := make([]store.ApplicationWithPosting, len(rows))
	for i, row := range rows {
		results[i] = store.ApplicationWithPosting{
			Application: toDomainApplication(sqlcgen.Application{
				ID: row.ID, TenantID: row.TenantID, PostingID: row.PostingID,
				Status: row.Status, AppliedAt: row.AppliedAt,
				ResumeVariant: row.ResumeVariant, ResumeUrl: row.ResumeUrl,
				ResumeHash: row.ResumeHash, CoverLetterVariant: row.CoverLetterVariant,
				NextActionAt: row.NextActionAt, Notes: row.Notes,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			}),
			PostingTitle:   row.Title,
			PostingURL:     row.CanonicalUrl,
			LocationBucket: row.LocationBucket,
			CompanyName:    row.CompanyName,
		}
	}
	return results, nil
}

func (s applicationStore) ListFollowupsDue(ctx context.Context, tenantID domain.ID, dueBefore time.Time, limit int) ([]store.ApplicationWithPosting, error) {
	rows, err := s.q.ListFollowupsDue(ctx, sqlcgen.ListFollowupsDueParams{
		TenantID:  tenantID,
		DueBefore: &dueBefore,
		RowLimit:  clampLimit(limit),
	})
	if err != nil {
		return nil, translate("list follow-ups", "application", err)
	}

	results := make([]store.ApplicationWithPosting, len(rows))
	for i, row := range rows {
		results[i] = store.ApplicationWithPosting{
			Application: toDomainApplication(sqlcgen.Application{
				ID: row.ID, TenantID: row.TenantID, PostingID: row.PostingID,
				Status: row.Status, AppliedAt: row.AppliedAt,
				ResumeVariant: row.ResumeVariant, ResumeUrl: row.ResumeUrl,
				ResumeHash: row.ResumeHash, CoverLetterVariant: row.CoverLetterVariant,
				NextActionAt: row.NextActionAt, Notes: row.Notes,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			}),
			PostingTitle: row.Title,
			PostingURL:   row.CanonicalUrl,
			CompanyName:  row.CompanyName,
		}
	}
	return results, nil
}

func (s applicationStore) CountByStatus(ctx context.Context, tenantID domain.ID) ([]store.StatusCount, error) {
	rows, err := s.q.CountApplicationsByStatus(ctx, tenantID)
	if err != nil {
		return nil, translate("count applications by status", "application", err)
	}
	counts := make([]store.StatusCount, len(rows))
	for i, row := range rows {
		counts[i] = store.StatusCount{Status: domain.Status(row.Status), Count: int(row.Total)}
	}
	return counts, nil
}

func (s applicationStore) ResumeVariantStats(ctx context.Context, tenantID domain.ID) ([]store.ResumeVariantStat, error) {
	rows, err := s.q.CountResponsesByResumeVariant(ctx, tenantID)
	if err != nil {
		return nil, translate("resume variant stats", "application", err)
	}
	stats := make([]store.ResumeVariantStat, len(rows))
	for i, row := range rows {
		stats[i] = store.ResumeVariantStat{
			Variant:   row.ResumeVariant,
			Sent:      int(row.Sent),
			Responded: int(row.Responded),
			Rejected:  int(row.Rejected),
			Ghosted:   int(row.Ghosted),
		}
	}
	return stats, nil
}
