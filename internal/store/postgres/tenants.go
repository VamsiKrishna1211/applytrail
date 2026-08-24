package postgres

import (
	"context"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

type tenantStore struct{ q *sqlcgen.Queries }

func (s tenantStore) Create(ctx context.Context, name, slug string) (domain.Tenant, error) {
	row, err := s.q.CreateTenant(ctx, sqlcgen.CreateTenantParams{Name: name, Slug: slug})
	if err != nil {
		return domain.Tenant{}, translate("create tenant", "tenant", err)
	}
	return toDomainTenant(row), nil
}

func (s tenantStore) Get(ctx context.Context, id domain.ID) (domain.Tenant, error) {
	row, err := s.q.GetTenant(ctx, id)
	if err != nil {
		return domain.Tenant{}, translate("get tenant", "tenant", err)
	}
	return toDomainTenant(row), nil
}

func (s tenantStore) GetBySlug(ctx context.Context, slug string) (domain.Tenant, error) {
	row, err := s.q.GetTenantBySlug(ctx, slug)
	if err != nil {
		return domain.Tenant{}, translate("get tenant by slug", "tenant", err)
	}
	return toDomainTenant(row), nil
}

func (s tenantStore) List(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := s.q.ListTenants(ctx)
	if err != nil {
		return nil, translate("list tenants", "tenant", err)
	}
	tenants := make([]domain.Tenant, len(rows))
	for i, row := range rows {
		tenants[i] = toDomainTenant(row)
	}
	return tenants, nil
}

type apiKeyStore struct{ q *sqlcgen.Queries }

func (s apiKeyStore) Create(ctx context.Context, key domain.APIKey) (domain.APIKey, error) {
	row, err := s.q.CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		TenantID:  key.TenantID,
		Prefix:    key.Prefix,
		Hash:      key.Hash,
		Label:     key.Label,
		Scopes:    fromScopes(key.Scopes),
		ExpiresAt: key.ExpiresAt,
	})
	if err != nil {
		return domain.APIKey{}, translate("create api key", "api key", err)
	}
	return toDomainAPIKey(row), nil
}

func (s apiKeyStore) GetByPrefix(ctx context.Context, prefix string) (domain.APIKey, error) {
	row, err := s.q.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return domain.APIKey{}, translate("get api key by prefix", "api key", err)
	}
	return toDomainAPIKey(row), nil
}

func (s apiKeyStore) List(ctx context.Context, tenantID domain.ID) ([]domain.APIKey, error) {
	rows, err := s.q.ListAPIKeys(ctx, tenantID)
	if err != nil {
		return nil, translate("list api keys", "api key", err)
	}
	keys := make([]domain.APIKey, len(rows))
	for i, row := range rows {
		keys[i] = toDomainAPIKey(row)
	}
	return keys, nil
}

func (s apiKeyStore) Touch(ctx context.Context, id domain.ID, usedAt time.Time) error {
	err := s.q.TouchAPIKey(ctx, sqlcgen.TouchAPIKeyParams{ID: id, UsedAt: &usedAt})
	return translate("touch api key", "api key", err)
}

func (s apiKeyStore) Revoke(ctx context.Context, tenantID, id domain.ID) (domain.APIKey, error) {
	row, err := s.q.RevokeAPIKey(ctx, sqlcgen.RevokeAPIKeyParams{ID: id, TenantID: tenantID})
	if err != nil {
		return domain.APIKey{}, translate("revoke api key", "api key", err)
	}
	return toDomainAPIKey(row), nil
}

type companyStore struct{ q *sqlcgen.Queries }

func (s companyStore) Upsert(ctx context.Context, company domain.Company) (domain.Company, error) {
	row, err := s.q.UpsertCompany(ctx, sqlcgen.UpsertCompanyParams{
		TenantID:       company.TenantID,
		Name:           company.Name,
		NormalizedName: company.NormalizedName,
		Domain:         company.Domain,
		CareersUrl:     company.CareersURL,
	})
	if err != nil {
		return domain.Company{}, translate("upsert company", "company", err)
	}
	return toDomainCompany(row), nil
}

func (s companyStore) Get(ctx context.Context, tenantID, id domain.ID) (domain.Company, error) {
	row, err := s.q.GetCompany(ctx, sqlcgen.GetCompanyParams{ID: id, TenantID: tenantID})
	if err != nil {
		return domain.Company{}, translate("get company", "company", err)
	}
	return toDomainCompany(row), nil
}

func (s companyStore) GetByNormalizedName(ctx context.Context, tenantID domain.ID, normalizedName string) (domain.Company, error) {
	row, err := s.q.GetCompanyByNormalizedName(ctx, sqlcgen.GetCompanyByNormalizedNameParams{
		TenantID:       tenantID,
		NormalizedName: normalizedName,
	})
	if err != nil {
		return domain.Company{}, translate("get company by name", "company", err)
	}
	return toDomainCompany(row), nil
}

func (s companyStore) List(ctx context.Context, tenantID domain.ID) ([]domain.Company, error) {
	rows, err := s.q.ListCompanies(ctx, tenantID)
	if err != nil {
		return nil, translate("list companies", "company", err)
	}
	companies := make([]domain.Company, len(rows))
	for i, row := range rows {
		companies[i] = toDomainCompany(row)
	}
	return companies, nil
}
