// Package testsupport provides shared fixtures for tests that need a real
// database.
//
// It lives outside _test.go files so several packages can use one container:
// starting Postgres per package would multiply an already slow step.
package testsupport

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/migrate"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres"
)

var (
	once      sync.Once
	sharedDSN string
	pool      *pgxpool.Pool
	startErr  error
)

// startContainerTimeout bounds the one-time bootstrap. Pulling the image on a
// cold machine is the slow part.
const startContainerTimeout = 3 * time.Minute

// Pool returns a migrated Postgres pool shared by every caller.
//
// The container starts on first use and lives for the duration of the test
// binary. Tests never tear down or reset the schema; they isolate by tenant,
// which is faster and exercises the tenant scoping the service depends on.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping test that needs a database; run without -short")
	}

	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), startContainerTimeout)
		defer cancel()

		container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("applytrail_test"),
			tcpostgres.WithUsername("applytrail"),
			tcpostgres.WithPassword("applytrail"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(90*time.Second),
			),
		)
		if err != nil {
			startErr = fmt.Errorf("start postgres container: %w", err)
			return
		}

		sharedDSN, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			startErr = fmt.Errorf("container connection string: %w", err)
			return
		}
		if err := migrate.Up(ctx, sharedDSN); err != nil {
			startErr = fmt.Errorf("apply migrations: %w", err)
			return
		}
		pool, err = postgres.NewPool(ctx, postgres.PoolConfig{URL: sharedDSN, MaxConns: 8})
		if err != nil {
			startErr = fmt.Errorf("open pool: %w", err)
		}
	})

	if startErr != nil {
		t.Fatalf("%v\n(run `go test -short ./...` to skip database tests)", startErr)
	}
	return pool
}

// Store returns a store and a tenant scoped to the calling test.
func Store(t *testing.T) (store.Store, domain.ID) {
	t.Helper()

	s := postgres.New(Pool(t))
	tenant, err := s.Tenants().Create(t.Context(), t.Name(), fmt.Sprintf("%s-%s", t.Name(), domain.NewID()))
	if err != nil {
		t.Fatalf("create tenant for test: %v", err)
	}
	return s, tenant.ID
}

// Principal returns an authenticated caller holding scopes, for a test that
// needs to call the service layer.
func Principal(tenantID domain.ID, scopes ...domain.Scope) domain.Principal {
	if len(scopes) == 0 {
		scopes = []domain.Scope{domain.ScopeRead, domain.ScopeWrite, domain.ScopeAdmin}
	}
	return domain.Principal{
		TenantID: tenantID,
		KeyID:    domain.NewID(),
		KeyLabel: "test",
		Scopes:   scopes,
	}
}
