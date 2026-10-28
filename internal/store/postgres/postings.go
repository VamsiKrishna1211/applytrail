package postgres

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

type postingStore struct{ q *sqlcgen.Queries }

// Each Find* method returns early on an empty input rather than issuing a
// query with an empty array: dedupe skips tiers whose keys nobody supplied,
// and that skip should not cost a round trip.

func (s postingStore) FindByCanonicalURLs(ctx context.Context, tenantID domain.ID, urls []string) ([]domain.PostingRef, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	rows, err := s.q.FindPostingsByCanonicalURLs(ctx, sqlcgen.FindPostingsByCanonicalURLsParams{
		TenantID:      tenantID,
		CanonicalUrls: urls,
	})
	if err != nil {
		return nil, translate("find postings by url", "posting", err)
	}
	refs := make([]domain.PostingRef, len(rows))
	for i, row := range rows {
		refs[i] = domain.PostingRef{
			PostingID:         row.ID,
			CompanyID:         row.CompanyID,
			CanonicalURL:      row.CanonicalUrl,
			Source:            domain.Source(row.Source),
			ExternalID:        row.ExternalID,
			Fingerprint:       row.Fingerprint,
			DescriptionHash:   row.DescriptionHash,
			FirstSeenAt:       row.FirstSeenAt,
			LastSeenAt:        row.LastSeenAt,
			ApplicationID:     row.ApplicationID,
			ApplicationStatus: toStatusPtr(row.ApplicationStatus),
		}
	}
	return refs, nil
}

func (s postingStore) FindByExternalIDs(ctx context.Context, tenantID domain.ID, keys []domain.SourceKey) ([]domain.PostingRef, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	composite := make([]string, len(keys))
	for i, k := range keys {
		composite[i] = k.String()
	}

	rows, err := s.q.FindPostingsByExternalIDs(ctx, sqlcgen.FindPostingsByExternalIDsParams{
		TenantID:          tenantID,
		SourceExternalIds: composite,
	})
	if err != nil {
		return nil, translate("find postings by external id", "posting", err)
	}
	refs := make([]domain.PostingRef, len(rows))
	for i, row := range rows {
		refs[i] = domain.PostingRef{
			PostingID:         row.ID,
			CompanyID:         row.CompanyID,
			CanonicalURL:      row.CanonicalUrl,
			Source:            domain.Source(row.Source),
			ExternalID:        row.ExternalID,
			Fingerprint:       row.Fingerprint,
			DescriptionHash:   row.DescriptionHash,
			FirstSeenAt:       row.FirstSeenAt,
			LastSeenAt:        row.LastSeenAt,
			ApplicationID:     row.ApplicationID,
			ApplicationStatus: toStatusPtr(row.ApplicationStatus),
		}
	}
	return refs, nil
}

func (s postingStore) FindByFingerprints(ctx context.Context, tenantID domain.ID, fingerprints []string) ([]domain.PostingRef, error) {
	if len(fingerprints) == 0 {
		return nil, nil
	}
	rows, err := s.q.FindPostingsByFingerprints(ctx, sqlcgen.FindPostingsByFingerprintsParams{
		TenantID:     tenantID,
		Fingerprints: fingerprints,
	})
	if err != nil {
		return nil, translate("find postings by fingerprint", "posting", err)
	}
	refs := make([]domain.PostingRef, len(rows))
	for i, row := range rows {
		refs[i] = domain.PostingRef{
			PostingID:         row.ID,
			CompanyID:         row.CompanyID,
			CanonicalURL:      row.CanonicalUrl,
			Source:            domain.Source(row.Source),
			ExternalID:        row.ExternalID,
			Fingerprint:       row.Fingerprint,
			DescriptionHash:   row.DescriptionHash,
			FirstSeenAt:       row.FirstSeenAt,
			LastSeenAt:        row.LastSeenAt,
			ApplicationID:     row.ApplicationID,
			ApplicationStatus: toStatusPtr(row.ApplicationStatus),
		}
	}
	return refs, nil
}

func (s postingStore) FindByDescriptionHashes(ctx context.Context, tenantID domain.ID, companyIDs []domain.ID, hashes []string) ([]domain.PostingRef, error) {
	if len(hashes) == 0 || len(companyIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q.FindPostingsByDescriptionHashes(ctx, sqlcgen.FindPostingsByDescriptionHashesParams{
		TenantID:          tenantID,
		CompanyIds:        companyIDs,
		DescriptionHashes: hashes,
	})
	if err != nil {
		return nil, translate("find postings by description hash", "posting", err)
	}
	refs := make([]domain.PostingRef, len(rows))
	for i, row := range rows {
		refs[i] = domain.PostingRef{
			PostingID:         row.ID,
			CompanyID:         row.CompanyID,
			CanonicalURL:      row.CanonicalUrl,
			Source:            domain.Source(row.Source),
			ExternalID:        row.ExternalID,
			Fingerprint:       row.Fingerprint,
			DescriptionHash:   row.DescriptionHash,
			FirstSeenAt:       row.FirstSeenAt,
			LastSeenAt:        row.LastSeenAt,
			ApplicationID:     row.ApplicationID,
			ApplicationStatus: toStatusPtr(row.ApplicationStatus),
		}
	}
	return refs, nil
}

func (s postingStore) FindFuzzyCandidates(ctx context.Context, tenantID, companyID domain.ID, normalizedTitle string, limit int) ([]domain.PostingRef, error) {
	if normalizedTitle == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := s.q.FindFuzzyPostingCandidates(ctx, sqlcgen.FindFuzzyPostingCandidatesParams{
		TenantID:        tenantID,
		CompanyID:       companyID,
		NormalizedTitle: normalizedTitle,
		RowLimit:        int32(limit), //nolint:gosec // limit is bounded by the caller
	})
	if err != nil {
		return nil, translate("find fuzzy posting candidates", "posting", err)
	}
	refs := make([]domain.PostingRef, len(rows))
	for i, row := range rows {
		refs[i] = domain.PostingRef{
			PostingID:         row.ID,
			CompanyID:         companyID,
			CanonicalURL:      row.CanonicalUrl,
			NormalizedTitle:   row.NormalizedTitle,
			LocationBucket:    row.LocationBucket,
			FirstSeenAt:       row.FirstSeenAt,
			LastSeenAt:        row.LastSeenAt,
			ApplicationID:     row.ApplicationID,
			ApplicationStatus: toStatusPtr(row.ApplicationStatus),
			Similarity:        float64(row.Score),
		}
	}
	return refs, nil
}

func (s postingStore) Insert(ctx context.Context, p domain.Posting) (domain.Posting, error) {
	seenAt := p.FirstSeenAt
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}

	row, err := s.q.InsertPosting(ctx, sqlcgen.InsertPostingParams{
		TenantID:        p.TenantID,
		CompanyID:       p.CompanyID,
		Source:          string(p.Source),
		ExternalID:      p.ExternalID,
		CanonicalUrl:    p.CanonicalURL,
		RawUrl:          p.RawURL,
		Title:           p.Title,
		NormalizedTitle: p.NormalizedTitle,
		LocationRaw:     p.LocationRaw,
		LocationBucket:  p.LocationBucket,
		RemoteType:      string(p.RemoteType),
		Seniority:       string(p.Seniority),
		CompMin:         p.Compensation.Min,
		CompMax:         p.Compensation.Max,
		CompCurrency:    p.Compensation.Currency,
		VisaSponsorship: p.VisaSponsorship,
		TechTags:        orEmptySlice(p.TechTags),
		DescriptionHash: p.DescriptionHash,
		Fingerprint:     p.Fingerprint,
		PostedAt:        p.PostedAt,
		SeenAt:          seenAt,
		Raw:             orEmptyJSON(p.Raw),
	})
	if err != nil {
		return domain.Posting{}, translate("insert posting", "posting", err)
	}
	return toDomainPosting(row), nil
}

func (s postingStore) Get(ctx context.Context, tenantID, id domain.ID) (domain.Posting, error) {
	row, err := s.q.GetPosting(ctx, sqlcgen.GetPostingParams{ID: id, TenantID: tenantID})
	if err != nil {
		return domain.Posting{}, translate("get posting", "posting", err)
	}
	return toDomainPosting(row), nil
}

func (s postingStore) TouchSeen(ctx context.Context, tenantID, id domain.ID, seenAt time.Time) (domain.Posting, error) {
	row, err := s.q.TouchPostingSeen(ctx, sqlcgen.TouchPostingSeenParams{
		ID:       id,
		TenantID: tenantID,
		SeenAt:   seenAt,
	})
	if err != nil {
		return domain.Posting{}, translate("touch posting", "posting", err)
	}
	return toDomainPosting(row), nil
}

func (s postingStore) Close(ctx context.Context, tenantID, id domain.ID, closedAt time.Time) error {
	err := s.q.ClosePosting(ctx, sqlcgen.ClosePostingParams{
		ID:       id,
		TenantID: tenantID,
		ClosedAt: &closedAt,
	})
	return translate("close posting", "posting", err)
}
