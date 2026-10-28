package clerkauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/go-jose/go-jose/v3"
	josejwt "github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

const (
	webOrigin = "https://applytrail.example.com"
	issuer    = "https://example.clerk.accounts.dev"
	keyID     = "ins_test_key"
)

type fakeKeySet struct {
	set   *clerk.JSONWebKeySet
	err   error
	calls int
}

func (f *fakeKeySet) Get(context.Context, *jwks.GetParams) (*clerk.JSONWebKeySet, error) {
	f.calls++
	return f.set, f.err
}

// signer mints session tokens the way Clerk does: RS256, with the key ID in
// the header and the session, subject and origin in the claims.
type signer struct {
	key *rsa.PrivateKey
	kid string
}

func newSigner(t *testing.T) signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return signer{key: key, kid: keyID}
}

func (s signer) publicKey() *clerk.JSONWebKey {
	return &clerk.JSONWebKey{Key: &s.key.PublicKey, KeyID: s.kid, Algorithm: "RS256", Use: "sig"}
}

type sessionClaims struct {
	josejwt.Claims
	SessionID       string `json:"sid,omitempty"`
	AuthorizedParty string `json:"azp,omitempty"`
}

func (s signer) token(t *testing.T, edit func(*sessionClaims)) string {
	t.Helper()
	now := time.Now()
	c := sessionClaims{
		Claims: josejwt.Claims{
			Issuer:    issuer,
			Subject:   owner,
			IssuedAt:  josejwt.NewNumericDate(now),
			NotBefore: josejwt.NewNumericDate(now.Add(-time.Second)),
			Expiry:    josejwt.NewNumericDate(now.Add(time.Minute)),
		},
		SessionID:       "sess_1",
		AuthorizedParty: webOrigin,
	}
	if edit != nil {
		edit(&c)
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", s.kid))
	require.NoError(t, err)
	raw, err := josejwt.Signed(sig).Claims(c).CompactSerialize()
	require.NoError(t, err)
	return raw
}

func newSessions(keys keySetAPI) *SessionTokens {
	o := options()
	return &SessionTokens{
		keys:    newKeyCache(keys),
		parties: map[string]bool{webOrigin: true},
		tenants: o.Tenants,
		log:     o.Logger,
	}
}

func TestSessionTokens(t *testing.T) {
	s := newSigner(t)
	keys := &fakeKeySet{set: &clerk.JSONWebKeySet{Keys: []*clerk.JSONWebKey{s.publicKey()}}}
	v := newSessions(keys)
	ctx := context.Background()

	p, err := v.Verify(ctx, s.token(t, nil))
	require.NoError(t, err)
	assert.Equal(t, tenant, p.TenantID)
	assert.Equal(t, credentialID("session", "sess_1"), p.KeyID)
	assert.ElementsMatch(t, []domain.Scope{domain.ScopeRead, domain.ScopeWrite}, p.Scopes)

	other := newSigner(t)
	rejected := map[string]string{
		"another origin":  s.token(t, func(c *sessionClaims) { c.AuthorizedParty = "https://evil.example" }),
		"no origin":       s.token(t, func(c *sessionClaims) { c.AuthorizedParty = "" }),
		"expired":         s.token(t, func(c *sessionClaims) { c.Expiry = josejwt.NewNumericDate(time.Now().Add(-time.Minute)) }),
		"unknown subject": s.token(t, func(c *sessionClaims) { c.Subject = "user_stranger" }),
		"foreign issuer":  s.token(t, func(c *sessionClaims) { c.Issuer = "https://evil.example" }),
		"forged":          other.token(t, nil),
		"garbage":         "eyJ.not.ajwt",
	}
	for name, token := range rejected {
		_, err := v.Verify(ctx, token)
		assert.ErrorIs(t, err, domain.ErrUnauthenticated, name)
	}

	// Every token above named the same key, so Clerk was asked once.
	assert.Equal(t, 1, keys.calls)
}

func TestSessionTokens_UnknownKeysDoNotHammerClerk(t *testing.T) {
	s := newSigner(t)
	keys := &fakeKeySet{set: &clerk.JSONWebKeySet{Keys: []*clerk.JSONWebKey{s.publicKey()}}}
	v := newSessions(keys)
	ctx := context.Background()

	rogue := signer{key: s.key, kid: "kid_rogue"}
	for range 3 {
		_, err := v.Verify(ctx, rogue.token(t, nil))
		assert.ErrorIs(t, err, domain.ErrUnauthenticated)
	}
	assert.Equal(t, 1, keys.calls, "an unknown key ID refetches at most once per interval")

	// Clerk rotates: once the interval passes, the new key is fetched.
	rotated := newSigner(t)
	rotated.kid = "kid_rotated"
	keys.set = &clerk.JSONWebKeySet{Keys: []*clerk.JSONWebKey{rotated.publicKey()}}
	v.keys.now = func() time.Time { return time.Now().Add(keyRefreshInterval + time.Second) }
	_, err := v.Verify(ctx, rotated.token(t, nil))
	require.NoError(t, err)
	assert.Equal(t, 2, keys.calls)
}

func TestSessionTokens_ClerkDown(t *testing.T) {
	s := newSigner(t)
	v := newSessions(&fakeKeySet{err: errors.New("connection refused")})
	_, err := v.Verify(context.Background(), s.token(t, nil))
	assert.ErrorIs(t, err, domain.ErrUnavailable)

	v = newSessions(&fakeKeySet{err: &clerk.APIErrorResponse{HTTPStatusCode: http.StatusUnauthorized}})
	_, err = v.Verify(context.Background(), s.token(t, nil))
	assert.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func TestTokenShapes(t *testing.T) {
	s := newSigner(t)
	session := s.token(t, nil)
	oauthJWT := s.token(t, func(c *sessionClaims) { c.SessionID = "" })

	assert.True(t, IsSessionToken(session))
	assert.False(t, IsOAuthToken(session), "a session token must not reach the OAuth verifier")

	assert.False(t, IsSessionToken(oauthJWT))
	assert.True(t, IsOAuthToken(oauthJWT))

	for _, other := range []string{"oat_abc", "ak_abc", "at_live_abc", "eyJ.x.y"} {
		assert.False(t, IsSessionToken(other), other)
	}
}
