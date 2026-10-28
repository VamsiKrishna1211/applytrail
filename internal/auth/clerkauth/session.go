package clerkauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/jwt"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// IsSessionToken matches a Clerk session token: the short-lived JWT Clerk's
// browser SDK mints for a signed-in user. It is told apart from an OAuth
// access token in JWT form by its session claim, which only session tokens
// carry. The payload is read without verification, which is safe because
// this only chooses the verifier; the verifier is what checks the signature.
func IsSessionToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || !strings.HasPrefix(token, "eyJ") {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		SessionID string `json:"sid"`
	}
	return json.Unmarshal(payload, &claims) == nil && claims.SessionID != ""
}

// keySetAPI is the Clerk call that fetches the instance's signing keys, so
// tests can stand in for Clerk.
type keySetAPI interface {
	Get(ctx context.Context, params *jwks.GetParams) (*clerk.JSONWebKeySet, error)
}

// SessionTokens verifies Clerk session tokens, which is how a person signs in
// to the web UI. Unlike the other two verifiers it does not ask Clerk about
// each token: a session token is a signed JWT that lives for a minute, so it
// is checked locally against Clerk's published signing keys.
type SessionTokens struct {
	keys    *keyCache
	parties map[string]bool
	tenants TenantResolver
	log     *slog.Logger
}

var _ auth.Verifier = (*SessionTokens)(nil)

// NewSessionTokens returns a verifier that accepts only tokens minted for a
// page served from one of authorizedParties, the origins the web UI is
// reached at. Clerk records the page's origin in each token, so a token
// obtained by some other site using the same Clerk instance is refused.
func NewSessionTokens(opts Options, authorizedParties []string) *SessionTokens {
	opts = opts.withDefaults()
	parties := make(map[string]bool, len(authorizedParties))
	for _, p := range authorizedParties {
		parties[strings.TrimSuffix(p, "/")] = true
	}
	return &SessionTokens{
		keys:    newKeyCache(&jwks.Client{Backend: opts.backend()}),
		parties: parties,
		tenants: opts.Tenants,
		log:     opts.Logger,
	}
}

// sessionLeeway absorbs clock skew between Clerk and this server. Session
// tokens live for sixty seconds, so it stays small.
const sessionLeeway = 5 * time.Second

// Verify implements auth.Verifier.
//
// The signed-in person acts for their tenant with read and write. Admin is
// never granted: the browser holds this token, and key management stays with
// ApplyTrail's own keys.
func (s *SessionTokens) Verify(ctx context.Context, token string) (domain.Principal, error) {
	unverified, err := jwt.Decode(ctx, &jwt.DecodeParams{Token: token})
	if err != nil {
		s.log.DebugContext(ctx, "clerk session token rejected", "reason", "malformed", "error", err)
		return domain.Principal{}, errInvalidCredential
	}
	key, err := s.keys.get(ctx, unverified.KeyID)
	if err != nil {
		return domain.Principal{}, classify(s.log, "clerk signing keys", err)
	}
	if key == nil {
		s.log.DebugContext(ctx, "clerk session token rejected", "reason", "unknown signing key", "kid", unverified.KeyID)
		return domain.Principal{}, errInvalidCredential
	}

	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{
		Token:                  token,
		JWK:                    key,
		Leeway:                 sessionLeeway,
		AuthorizedPartyHandler: func(azp string) bool { return s.parties[azp] },
	})
	if err != nil {
		s.log.DebugContext(ctx, "clerk session token rejected", "reason", err.Error())
		return domain.Principal{}, errInvalidCredential
	}
	tenant, known := s.tenants.TenantFor(ctx, claims.Subject)
	if !known {
		s.log.DebugContext(ctx, "clerk session token rejected", "reason", "subject maps to no tenant", "session", claims.SessionID)
		return domain.Principal{}, errInvalidCredential
	}

	return domain.Principal{
		TenantID: tenant,
		// One identity per browser session, so each signed-in tab or device
		// gets its own rate-limit budget and its own line in the request log.
		KeyID:    credentialID("session", claims.SessionID),
		KeyLabel: "clerk-session",
		Scopes:   []domain.Scope{domain.ScopeRead, domain.ScopeWrite},
	}, nil
}

// keyRefreshInterval is the most often the signing keys are refetched. Clerk
// rotates them rarely; an unknown key ID usually means a forged token, and
// refetching on every one would let anyone make this server call Clerk at
// will.
const keyRefreshInterval = 5 * time.Minute

// keyCache holds the instance's signing keys by key ID, refetching when a
// token names a key it has not seen and the last fetch is old enough.
type keyCache struct {
	api keySetAPI
	now func() time.Time

	mu      sync.Mutex
	keys    map[string]*clerk.JSONWebKey
	fetched time.Time
}

func newKeyCache(api keySetAPI) *keyCache {
	return &keyCache{api: api, now: time.Now, keys: map[string]*clerk.JSONWebKey{}}
}

// get returns the key for kid, or nil when Clerk does not publish one.
func (c *keyCache) get(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	if !c.fetched.IsZero() && c.now().Sub(c.fetched) < keyRefreshInterval {
		return nil, nil
	}
	set, err := c.api.Get(ctx, &jwks.GetParams{})
	if err != nil {
		return nil, err
	}
	c.fetched = c.now()
	c.keys = make(map[string]*clerk.JSONWebKey, len(set.Keys))
	for _, k := range set.Keys {
		if k != nil {
			c.keys[k.KeyID] = k
		}
	}
	return c.keys[kid], nil
}
