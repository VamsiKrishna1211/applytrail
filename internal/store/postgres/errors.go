package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Postgres error codes worth translating rather than surfacing as 500s.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
)

// translate maps a driver error onto a domain sentinel so transports can pick
// the right status without knowing about Postgres. Anything unrecognised is
// wrapped with context and stays a 500, which is correct: an unexpected
// database error is a bug, not a client mistake.
func translate(op string, kind string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, domain.NewNotFoundError(kind, ""))
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%s: %w: %s already exists (%s)", op, domain.ErrConflict, kind, pgErr.ConstraintName)
		case pgForeignKeyViolation:
			return fmt.Errorf("%s: %w: referenced record does not exist (%s)", op, domain.ErrInvalidArgument, pgErr.ConstraintName)
		case pgCheckViolation:
			return fmt.Errorf("%s: %w: violates %s", op, domain.ErrInvalidArgument, pgErr.ConstraintName)
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// notFound builds a not-found error naming the specific record.
func notFound(kind string, id fmt.Stringer) error {
	return domain.NewNotFoundError(kind, id.String())
}
