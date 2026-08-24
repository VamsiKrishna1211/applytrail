package apikey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
)

// errInvalidCredential is the single message every failure path returns.
//
// Distinguishing "no such key" from "revoked key" from "wrong key" would tell
// an attacker which prefixes exist, so all four cases look identical from
// outside. The real reason is logged at debug level for the operator.
var errInvalidCredential = fmt.Errorf("%w: missing or invalid API key", domain.ErrUnauthenticated)

// Authenticator resolves bearer tokens to principals.
type Authenticator struct {
	keys store.APIKeyStore
	now  func() time.Time

	// touch records last-used timestamps. It is best-effort: a failure to
	// record usage must never fail an otherwise valid request.
	touch func(ctx context.Context, id domain.ID, at time.Time)
}

var _ auth.Authenticator = (*Authenticator)(nil)

// New returns an Authenticator backed by keys.
func New(keys store.APIKeyStore) *Authenticator {
	a := &Authenticator{keys: keys, now: time.Now}
	a.touch = a.recordUsage
	return a
}

// WithClock overrides the time source, for tests around expiry.
func (a *Authenticator) WithClock(now func() time.Time) *Authenticator {
	a.now = now
	return a
}

// Authenticate resolves the request's bearer token.
func (a *Authenticator) Authenticate(ctx context.Context, r *http.Request) (domain.Principal, error) {
	secret, ok := bearerToken(r)
	if !ok {
		return domain.Principal{}, errInvalidCredential
	}

	prefix, ok := LookupPrefix(secret)
	if !ok {
		// Not even shaped like one of ours; no database round trip needed.
		return domain.Principal{}, errInvalidCredential
	}

	record, err := a.keys.GetByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Principal{}, errInvalidCredential
		}
		return domain.Principal{}, fmt.Errorf("look up api key: %w", err)
	}

	now := a.now()
	switch {
	case !Verify(secret, record.Hash):
		slog.DebugContext(ctx, "api key rejected", "reason", "hash mismatch", "prefix", prefix)
		return domain.Principal{}, errInvalidCredential
	case record.RevokedAt != nil:
		slog.DebugContext(ctx, "api key rejected", "reason", "revoked", "prefix", prefix)
		return domain.Principal{}, errInvalidCredential
	case !record.Active(now):
		slog.DebugContext(ctx, "api key rejected", "reason", "expired", "prefix", prefix)
		return domain.Principal{}, errInvalidCredential
	}

	a.touch(ctx, record.ID, now)

	return domain.Principal{
		TenantID: record.TenantID,
		KeyID:    record.ID,
		KeyLabel: record.Label,
		Scopes:   record.Scopes,
	}, nil
}

// recordUsage writes last_used_at without blocking the request on it. The
// context is detached because the request's context is cancelled as soon as the
// response is written.
func (a *Authenticator) recordUsage(ctx context.Context, id domain.ID, at time.Time) {
	detached := context.WithoutCancel(ctx)
	if err := a.keys.Touch(detached, id, at); err != nil {
		slog.WarnContext(ctx, "record api key usage", "error", err, "key_id", id)
	}
}

// bearerToken extracts the credential from the Authorization header.
//
// The scheme match is case-insensitive because clients send "Bearer", "bearer",
// and occasionally "BEARER", and rejecting those would be a confusing failure
// for no benefit.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}
