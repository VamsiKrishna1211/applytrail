package clerkauth

import (
	"context"
	"log/slog"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/apikey"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// APIKeyPrefix starts every Clerk API key secret.
const APIKeyPrefix = "ak_"

// The Clerk scopes a key can carry to narrow what it may do here.
const (
	ScopeRead  = "applytrail:read"
	ScopeWrite = "applytrail:write"
)

// keyAPI is the slice of Clerk's API this verifier calls, so tests can stand
// in for Clerk.
type keyAPI interface {
	Verify(ctx context.Context, params *apikey.VerifyParams) (*clerk.APIKey, error)
}

// APIKeys verifies Clerk API keys.
type APIKeys struct {
	api     keyAPI
	tenants TenantResolver
	cache   *cache
	log     *slog.Logger
}

var _ auth.Verifier = (*APIKeys)(nil)

// NewAPIKeys returns a verifier backed by Clerk's Backend API.
func NewAPIKeys(opts Options) *APIKeys {
	opts = opts.withDefaults()
	return &APIKeys{
		api:     &apikey.Client{Backend: opts.backend()},
		tenants: opts.Tenants,
		cache:   newCache(opts.CacheTTL),
		log:     opts.Logger,
	}
}

// Verify implements auth.Verifier.
func (k *APIKeys) Verify(ctx context.Context, token string) (domain.Principal, error) {
	if p, ok := k.cache.get(token); ok {
		return p, nil
	}
	key, err := k.api.Verify(ctx, &apikey.VerifyParams{Secret: token})
	if err != nil {
		return domain.Principal{}, classify(k.log, "clerk api key", err)
	}

	tenant, known := k.tenants.TenantFor(ctx, key.Subject)
	scopes, ours := keyScopes(key.Scopes)
	switch {
	case key.Revoked || key.Expired:
		k.log.DebugContext(ctx, "clerk api key rejected", "reason", "revoked or expired", "key", key.ID)
		return domain.Principal{}, errInvalidCredential
	case !known:
		k.log.DebugContext(ctx, "clerk api key rejected", "reason", "subject maps to no tenant", "key", key.ID)
		return domain.Principal{}, errInvalidCredential
	case !ours:
		k.log.DebugContext(ctx, "clerk api key rejected", "reason", "scoped for another service", "key", key.ID)
		return domain.Principal{}, errInvalidCredential
	}

	p := domain.Principal{
		TenantID: tenant,
		KeyID:    credentialID("api_key", key.ID),
		KeyLabel: "clerk:" + key.Name,
		Scopes:   scopes,
	}
	var notAfter time.Time
	if key.Expiration != nil {
		notAfter = unixTime(*key.Expiration)
	}
	k.cache.put(token, p, notAfter)
	return p, nil
}

// keyScopes maps a key's Clerk scopes onto ApplyTrail's. A key with no scopes
// is its owner's general-purpose key, and the prebuilt Clerk UI creates keys
// like that, so it gets read and write. A key with scopes but none of ours was
// minted for another service and is refused. Admin is never granted: key and
// profile management stays with ApplyTrail's own keys.
func keyScopes(granted []string) ([]domain.Scope, bool) {
	if len(granted) == 0 {
		return []domain.Scope{domain.ScopeRead, domain.ScopeWrite}, true
	}
	var scopes []domain.Scope
	for _, g := range granted {
		switch g {
		case ScopeRead:
			scopes = append(scopes, domain.ScopeRead)
		case ScopeWrite:
			scopes = append(scopes, domain.ScopeWrite)
		}
	}
	return scopes, len(scopes) > 0
}
