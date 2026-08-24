package postgres_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/migrate"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres"
)

// One container and one migrated schema serve the whole package. Tests isolate
// from each other by working under their own tenant rather than by resetting
// the database, which is both faster and a genuine exercise of the tenant
// scoping every query relies on.
var sharedPool *pgxpool.Pool

func TestMain(m *testing.M) {
	// testing.Short() reads a flag, so the flags must be parsed first: TestMain
	// runs before the testing package would do it itself.
	flag.Parse()

	if testing.Short() {
		// -short means "no Docker": skip the container entirely rather than
		// failing on a machine that has no daemon.
		os.Exit(m.Run())
	}

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("applytrail_test"),
		tcpostgres.WithUsername("applytrail"),
		tcpostgres.WithPassword("applytrail"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		fmt.Fprintln(os.Stderr, "run `go test -short ./...` to skip database tests")
		os.Exit(1)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminate container: %v\n", err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "container connection string: %v\n", err)
		os.Exit(1)
	}

	if err := migrate.Up(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "apply migrations: %v\n", err)
		os.Exit(1)
	}

	sharedPool, err = postgres.NewPool(ctx, postgres.PoolConfig{URL: dsn, MaxConns: 8})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	sharedPool.Close()
	os.Exit(code)
}

// newTestStore returns a store and a tenant scoped to the calling test.
func newTestStore(t *testing.T) (store.Store, domain.ID) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in -short mode")
	}

	s := postgres.New(sharedPool)
	tenant, err := s.Tenants().Create(t.Context(), t.Name(), fmt.Sprintf("%s-%s", t.Name(), domain.NewID()))
	require.NoError(t, err, "create tenant for test")
	return s, tenant.ID
}

// seedPosting inserts a company and a posting, returning the posting.
func seedPosting(t *testing.T, s store.Store, tenantID domain.ID, p domain.Posting) domain.Posting {
	t.Helper()
	ctx := t.Context()

	company, err := s.Companies().Upsert(ctx, domain.Company{
		TenantID:       tenantID,
		Name:           "Acme, Inc.",
		NormalizedName: "acme",
	})
	require.NoError(t, err)

	p.TenantID = tenantID
	p.CompanyID = company.ID
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
	}
	inserted, err := s.Postings().Insert(ctx, p)
	require.NoError(t, err)
	return inserted
}
