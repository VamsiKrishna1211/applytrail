package httpapi

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// handlers bundles the service the REST layer translates for. Every method on
// it decodes, calls one service method, and encodes; anything more than that
// belongs in internal/service.
type handlers struct {
	svc *service.Service
}

// encodeCursor renders a keyset position as an opaque token.
//
// Opaque matters: clients that parse a cursor start depending on its shape,
// and then the pagination key can never change.
func encodeCursor(c store.Cursor) string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(token string) (*store.Cursor, error) {
	invalid := domain.NewValidationError("cursor is not a valid pagination token; omit it to start from the beginning")

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, invalid
	}
	timestamp, id, found := strings.Cut(string(raw), "|")
	if !found {
		return nil, invalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return nil, invalid
	}
	parsedID, err := domain.ParseID("cursor", id)
	if err != nil {
		return nil, invalid
	}
	return &store.Cursor{CreatedAt: createdAt, ID: parsedID}, nil
}
