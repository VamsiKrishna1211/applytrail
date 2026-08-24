package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/store/postgres/sqlcgen"
)

// Store is the Postgres implementation of the persistence ports.
type Store struct {
	// pool is nil for a transaction-bound Store; q is always set.
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// compile-time proof the adapter satisfies the port.
var _ store.Store = (*Store)(nil)

// New returns a Store backed by pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlcgen.New(pool)}
}

// Tx runs fn inside a transaction, passing a Store bound to it.
//
// The rollback is deferred rather than called on each error path so a panic
// inside fn cannot leave the transaction open.
func (s *Store) Tx(ctx context.Context, fn func(ctx context.Context, tx store.Store) error) error {
	if s.pool == nil {
		return fmt.Errorf("nested transactions are not supported")
	}

	pgTx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// A no-op once the transaction has been committed.
		_ = pgTx.Rollback(context.WithoutCancel(ctx))
	}()

	if err := fn(ctx, &Store{q: s.q.WithTx(pgTx)}); err != nil {
		return err
	}
	if err := pgTx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) Tenants() store.TenantStore               { return tenantStore{q: s.q} }
func (s *Store) APIKeys() store.APIKeyStore               { return apiKeyStore{q: s.q} }
func (s *Store) Companies() store.CompanyStore            { return companyStore{q: s.q} }
func (s *Store) Postings() store.PostingStore             { return postingStore{q: s.q} }
func (s *Store) Applications() store.ApplicationStore     { return applicationStore{q: s.q} }
func (s *Store) Events() store.EventStore                 { return eventStore{q: s.q} }
func (s *Store) Contacts() store.ContactStore             { return contactStore{q: s.q} }
func (s *Store) SearchProfiles() store.SearchProfileStore { return searchProfileStore{q: s.q} }
func (s *Store) SearchRuns() store.SearchRunStore         { return searchRunStore{q: s.q} }
