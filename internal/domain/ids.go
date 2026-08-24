package domain

import "github.com/google/uuid"

// ID is the identifier type for every entity. UUIDs let a client generate an
// ID before a write, which is what makes ingest retries idempotent.
type ID = uuid.UUID

// NewID returns a random identifier.
func NewID() ID { return uuid.New() }

// ParseID parses a textual identifier, wrapping failures as invalid argument
// so transports map them to 400 rather than 500.
func ParseID(kind, s string) (ID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, NewValidationError(kind + " id must be a UUID, got " + quote(s))
	}
	return id, nil
}

func quote(s string) string {
	const max = 64
	if len(s) > max {
		s = s[:max] + "…"
	}
	return `"` + s + `"`
}
