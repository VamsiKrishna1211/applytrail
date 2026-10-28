package clerkauth

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// OAuthTokenPrefix starts every opaque Clerk OAuth access token.
const OAuthTokenPrefix = "oat_"

// IsOAuthToken matches the two forms a Clerk OAuth access token takes, chosen
// per instance in the Clerk dashboard: opaque (oat_...) or a JWT. Clerk's
// verify endpoint accepts both, so one verifier serves either setting. A JWT
// that is a session token belongs to SessionTokens instead.
func IsOAuthToken(token string) bool {
	return strings.HasPrefix(token, OAuthTokenPrefix) ||
		(strings.HasPrefix(token, "eyJ") && strings.Count(token, ".") == 2 && !IsSessionToken(token))
}

// tokenAPI is the Clerk call this verifier makes, so tests can stand in for
// Clerk. The Go SDK does not wrap this endpoint.
type tokenAPI interface {
	verify(ctx context.Context, token string) (*verifiedToken, error)
}

// verifiedToken is Clerk's answer, per the Backend API's
// verifyOAuthAccessToken operation. A JWT that has expired comes back as
// {"active": false} with no other fields.
type verifiedToken struct {
	clerk.APIResource
	Active     *bool    `json:"active,omitempty"`
	ID         string   `json:"id"`
	ClientID   string   `json:"client_id"`
	Subject    string   `json:"subject"`
	Scopes     []string `json:"scopes"`
	Audience   []string `json:"aud"`
	Revoked    bool     `json:"revoked"`
	Expired    bool     `json:"expired"`
	Expiration *float64 `json:"expiration"`
}

type verifyParams struct {
	clerk.APIParams
	AccessToken string `json:"access_token"`
}

type backendTokenAPI struct{ backend clerk.Backend }

func (b backendTokenAPI) verify(ctx context.Context, token string) (*verifiedToken, error) {
	req := clerk.NewAPIRequest(http.MethodPost, "/oauth_applications/access_tokens/verify")
	req.SetParams(&verifyParams{AccessToken: token})
	out := &verifiedToken{}
	err := b.backend.Call(ctx, req, out)
	return out, err
}

// OAuthTokens verifies OAuth access tokens that Clerk issued to MCP clients.
type OAuthTokens struct {
	api      tokenAPI
	resource string
	tenants  TenantResolver
	cache    *cache
	log      *slog.Logger
}

var _ auth.Verifier = (*OAuthTokens)(nil)

// NewOAuthTokens returns a verifier that accepts only tokens whose audience
// includes resource — this server's MCP URL, exactly as registered in Clerk.
// That is what stops a token Clerk minted for some other server on the same
// instance from working here.
func NewOAuthTokens(opts Options, resource string) *OAuthTokens {
	opts = opts.withDefaults()
	return &OAuthTokens{
		api:      backendTokenAPI{backend: opts.backend()},
		resource: resource,
		tenants:  opts.Tenants,
		cache:    newCache(opts.CacheTTL),
		log:      opts.Logger,
	}
}

// Verify implements auth.Verifier.
//
// A person connected Claude to ApplyTrail by signing in, so the token acts
// for that person's tenant with read and write. Clerk OAuth carries no custom
// scopes to narrow it, and admin is never granted.
func (o *OAuthTokens) Verify(ctx context.Context, token string) (domain.Principal, error) {
	if p, ok := o.cache.get(token); ok {
		return p, nil
	}
	t, err := o.api.verify(ctx, token)
	if err != nil {
		return domain.Principal{}, classify(o.log, "clerk oauth token", err)
	}

	tenant, known := o.tenants.TenantFor(ctx, t.Subject)
	switch {
	case t.Active != nil && !*t.Active, t.Revoked, t.Expired:
		o.log.DebugContext(ctx, "clerk oauth token rejected", "reason", "inactive, revoked or expired", "token", t.ID)
		return domain.Principal{}, errInvalidCredential
	case !slices.Contains(t.Audience, o.resource):
		o.log.DebugContext(ctx, "clerk oauth token rejected", "reason", "audience", "aud", t.Audience, "token", t.ID)
		return domain.Principal{}, errInvalidCredential
	case !known:
		o.log.DebugContext(ctx, "clerk oauth token rejected", "reason", "subject maps to no tenant", "token", t.ID)
		return domain.Principal{}, errInvalidCredential
	}

	p := domain.Principal{
		TenantID: tenant,
		// One identity per signed-in client, so each connected app gets its
		// own rate-limit budget and its own name in the request log.
		KeyID:    credentialID("oauth", t.ClientID+":"+t.Subject),
		KeyLabel: "clerk-oauth:" + t.ClientID,
		Scopes:   []domain.Scope{domain.ScopeRead, domain.ScopeWrite},
	}
	var notAfter time.Time
	if t.Expiration != nil {
		notAfter = unixTime(int64(*t.Expiration))
	}
	o.cache.put(token, p, notAfter)
	return p, nil
}

// unixTime reads a Clerk timestamp. Clerk's objects mostly carry
// milliseconds, but its published examples show seconds, so both are read:
// any value past 1e12 is milliseconds (1e12 seconds is the year 33658).
func unixTime(n int64) time.Time {
	if n > 1e12 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
