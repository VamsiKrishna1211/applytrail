package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres"
)

// defaultDatabaseURL matches deploy/docker-compose.yml, so the CLI works
// against a freshly started local stack with no configuration at all. The
// credentials in it are the compose file's throwaway local ones.
const defaultDatabaseURL = "postgres://applytrail:applytrail@localhost:5432/applytrail?sslmode=disable" //nolint:gosec // local development default, not a secret

// dbFlags adds the shared --database-url flag to a command's flag set.
func dbFlags(fs *flag.FlagSet) *string {
	fallback := os.Getenv("APPLYTRAIL_DATABASE_URL")
	if fallback == "" {
		fallback = defaultDatabaseURL
	}
	return fs.String("database-url", fallback, "postgres connection string")
}

// withStore opens a pool, runs fn, and closes it.
func withStore(ctx context.Context, databaseURL string, fn func(context.Context, store.Store) error) error {
	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{URL: databaseURL, MaxConns: 4})
	if err != nil {
		return err
	}
	defer pool.Close()
	return fn(ctx, postgres.New(pool))
}

var errNoTenant = errors.New(
	"no tenant exists yet; run `applytrail bootstrap` to create one")

// resolveTenant finds the tenant to operate on.
//
// A deployment normally holds exactly one, so requiring --tenant on every
// command would be friction for no benefit. The flag is only needed once a
// second tenant exists, and the error then says so.
func resolveTenant(ctx context.Context, st store.Store, slug string) (domain.Tenant, error) {
	if slug != "" {
		return st.Tenants().GetBySlug(ctx, slug)
	}

	tenants, err := st.Tenants().List(ctx)
	if err != nil {
		return domain.Tenant{}, err
	}
	switch len(tenants) {
	case 0:
		return domain.Tenant{}, errNoTenant
	case 1:
		return tenants[0], nil
	default:
		slugs := make([]string, len(tenants))
		for i, tenant := range tenants {
			slugs[i] = tenant.Slug
		}
		return domain.Tenant{}, fmt.Errorf(
			"this deployment holds %d tenants; pass --tenant with one of %v", len(tenants), slugs)
	}
}

// adminPrincipal builds the principal the CLI acts as.
//
// The CLI already requires database credentials, so gating it behind an API
// key would add a step without adding a boundary.
func adminPrincipal(tenantID domain.ID) domain.Principal {
	return domain.Principal{
		TenantID: tenantID,
		KeyID:    domain.NewID(),
		KeyLabel: "cli",
		Scopes:   []domain.Scope{domain.ScopeAdmin},
	}
}
