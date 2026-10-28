// Package clerkauth verifies credentials issued by Clerk: API keys a user
// mints from their Clerk profile, and OAuth access tokens Clerk issues to MCP
// clients such as Claude.
//
// It fills the place internal/auth/oidc marked out. Both verifiers implement
// auth.Verifier and return an ordinary domain.Principal, so nothing past the
// authentication middleware knows Clerk exists.
package clerkauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/google/uuid"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// DefaultCacheTTL is how long a verified credential is trusted without asking
// Clerk again. It is also the longest a revoked credential keeps working.
const DefaultCacheTTL = time.Minute

// errInvalidCredential is the one message every rejection returns, matching
// the API key authenticator: the reason goes to the debug log, never to the
// caller, so a rejection cannot be used to probe which credentials exist.
var errInvalidCredential = fmt.Errorf("%w: missing or invalid API key", domain.ErrUnauthenticated)

// errUnavailable tells the caller to retry rather than discard a credential
// that may be perfectly good.
var errUnavailable = fmt.Errorf("%w: could not verify the credential with the identity provider", domain.ErrUnavailable)

// TenantResolver maps a Clerk subject (user_... or org_...) to the tenant it
// acts for. It is also the allowlist: a subject it does not know is refused,
// so an account that can sign in to the Clerk instance does not thereby get
// into ApplyTrail.
type TenantResolver interface {
	TenantFor(ctx context.Context, subject string) (domain.ID, bool)
}

// StaticTenants is a TenantResolver over a fixed map, read from
// configuration. A database-backed resolver can replace it without touching
// either verifier.
type StaticTenants map[string]domain.ID

// TenantFor implements TenantResolver.
func (s StaticTenants) TenantFor(_ context.Context, subject string) (domain.ID, bool) {
	id, ok := s[subject]
	return id, ok
}

// ParseTenants reads "user_abc=<tenant uuid>,org_def=<tenant uuid>".
func ParseTenants(raw string) (StaticTenants, error) {
	tenants := StaticTenants{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		subject, id, ok := strings.Cut(pair, "=")
		subject, id = strings.TrimSpace(subject), strings.TrimSpace(id)
		if !ok || subject == "" {
			return nil, fmt.Errorf("%q is not subject=tenant-id", pair)
		}
		tenant, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("%q: tenant id must be a UUID", pair)
		}
		tenants[subject] = tenant
	}
	return tenants, nil
}

// Options configure both verifiers.
type Options struct {
	// SecretKey is the Clerk instance's secret key (sk_...).
	SecretKey string
	// APIURL overrides Clerk's Backend API base URL. Empty uses Clerk's.
	APIURL  string
	Tenants TenantResolver
	// CacheTTL bounds how long a successful check is reused. Zero means
	// DefaultCacheTTL.
	CacheTTL time.Duration
	Logger   *slog.Logger
}

func (o Options) backend() clerk.Backend {
	cfg := &clerk.BackendConfig{
		Key:        clerk.String(o.SecretKey),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	if o.APIURL != "" {
		cfg.URL = clerk.String(o.APIURL)
	}
	return clerk.NewBackend(cfg)
}

func (o Options) withDefaults() Options {
	if o.CacheTTL <= 0 {
		o.CacheTTL = DefaultCacheTTL
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// classify separates "Clerk says no" from "Clerk did not answer". Only the
// first is the caller's fault, and only it may spend the failed-auth budget.
func classify(log *slog.Logger, what string, err error) error {
	var apiErr *clerk.APIErrorResponse
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode >= 400 && apiErr.HTTPStatusCode < 500 {
		log.Debug(what+" rejected by clerk", "status", apiErr.HTTPStatusCode, "trace", apiErr.TraceID)
		return errInvalidCredential
	}
	log.Error(what+" verification failed", "error", err)
	return errUnavailable
}

// credentialID derives a stable ID for a Clerk credential, so the per-
// credential rate limiter and the request log treat it like an ApplyTrail key.
func credentialID(kind, id string) domain.ID {
	return uuid.NewSHA1(namespace, []byte(kind+":"+id))
}

// namespace scopes credentialID's UUIDs to this package.
var namespace = uuid.MustParse("6f1c2a52-58a9-4d0c-9b1e-0f6c5d1e9a77")

// cache remembers successful verifications under a hash of the credential,
// never the credential itself, so a memory dump hands out no working secrets.
type cache struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[[sha256.Size]byte]cacheEntry
}

type cacheEntry struct {
	principal domain.Principal
	expires   time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, now: time.Now, entries: map[[sha256.Size]byte]cacheEntry{}}
}

func (c *cache) get(token string) (domain.Principal, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[sha256.Sum256([]byte(token))]
	if !ok || !c.now().Before(e.expires) {
		return domain.Principal{}, false
	}
	return e.principal, true
}

// put stores p, expiring no later than notAfter when that is set, so a cached
// credential never outlives its own expiry.
func (c *cache) put(token string, p domain.Principal, notAfter time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	expires := now.Add(c.ttl)
	if !notAfter.IsZero() && notAfter.Before(expires) {
		expires = notAfter
	}
	c.entries[sha256.Sum256([]byte(token))] = cacheEntry{principal: p, expires: expires}
}

// FrontendAPI returns the origin of the instance's Frontend API, which the
// browser SDK loads from and talks to. A publishable key is that host, base64
// encoded and terminated by "$", behind a pk_test_ or pk_live_ prefix.
func FrontendAPI(publishableKey string) (string, error) {
	encoded, ok := strings.CutPrefix(publishableKey, "pk_test_")
	if !ok {
		encoded, ok = strings.CutPrefix(publishableKey, "pk_live_")
	}
	if !ok {
		return "", errors.New("not a Clerk publishable key")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil {
		return "", fmt.Errorf("publishable key does not decode: %w", err)
	}
	host, ok := strings.CutSuffix(string(decoded), "$")
	if !ok || host == "" || strings.ContainsAny(host, "/ ") {
		return "", errors.New("publishable key does not name a host")
	}
	return "https://" + host, nil
}
