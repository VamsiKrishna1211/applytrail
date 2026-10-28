package clerkauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/apikey"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

const (
	owner    = "user_owner"
	resource = "https://applytrail.example.com/mcp"
)

var tenant = uuid.MustParse("11111111-2222-3333-4444-555555555555")

func options() Options {
	return Options{
		Tenants: StaticTenants{owner: tenant},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}.withDefaults()
}

func TestParseTenants(t *testing.T) {
	got, err := ParseTenants(" user_a=" + tenant.String() + ", ,org_b=" + tenant.String())
	require.NoError(t, err)
	assert.Equal(t, StaticTenants{"user_a": tenant, "org_b": tenant}, got)

	for _, bad := range []string{"user_a", "=" + tenant.String(), "user_a=not-a-uuid"} {
		_, err := ParseTenants(bad)
		assert.Error(t, err, bad)
	}
}

// --- API keys ----------------------------------------------------------------

type fakeKeys struct {
	keys  map[string]*clerk.APIKey
	err   error
	calls int
}

func (f *fakeKeys) Verify(_ context.Context, p *apikey.VerifyParams) (*clerk.APIKey, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if k, ok := f.keys[p.Secret]; ok {
		return k, nil
	}
	return nil, &clerk.APIErrorResponse{HTTPStatusCode: http.StatusNotFound}
}

func newKeys(f *fakeKeys) *APIKeys {
	o := options()
	return &APIKeys{api: f, tenants: o.Tenants, cache: newCache(o.CacheTTL), log: o.Logger}
}

func TestAPIKeys(t *testing.T) {
	f := &fakeKeys{keys: map[string]*clerk.APIKey{
		"ak_all":      {ID: "1", Name: "routine", Subject: owner},
		"ak_read":     {ID: "2", Subject: owner, Scopes: []string{ScopeRead}},
		"ak_foreign":  {ID: "3", Subject: owner, Scopes: []string{"resumes:write"}},
		"ak_stranger": {ID: "4", Subject: "user_stranger"},
		"ak_revoked":  {ID: "5", Subject: owner, Revoked: true},
	}}
	v := newKeys(f)

	p, err := v.Verify(t.Context(), "ak_all")
	require.NoError(t, err)
	assert.Equal(t, tenant, p.TenantID)
	assert.Equal(t, "clerk:routine", p.KeyLabel)
	assert.True(t, p.HasScope(domain.ScopeRead) && p.HasScope(domain.ScopeWrite))
	assert.False(t, p.HasScope(domain.ScopeAdmin), "a Clerk key must never hold admin")

	p, err = v.Verify(t.Context(), "ak_read")
	require.NoError(t, err)
	assert.True(t, p.HasScope(domain.ScopeRead))
	assert.False(t, p.HasScope(domain.ScopeWrite))

	for _, bad := range []string{"ak_foreign", "ak_stranger", "ak_revoked", "ak_unknown"} {
		_, err := v.Verify(t.Context(), bad)
		assert.True(t, errors.Is(err, domain.ErrUnauthenticated), "%s: %v", bad, err)
	}
}

func TestAPIKeyOutageIsUnavailable(t *testing.T) {
	for _, cause := range []error{errors.New("dial tcp: i/o timeout"), &clerk.APIErrorResponse{HTTPStatusCode: 502}} {
		_, err := newKeys(&fakeKeys{err: cause}).Verify(t.Context(), "ak_x")
		assert.True(t, errors.Is(err, domain.ErrUnavailable), "%v", cause)
		assert.False(t, errors.Is(err, domain.ErrUnauthenticated), "an outage must not spend the failed-auth budget")
	}
}

func TestCacheReusesSuccessAndRespectsExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	soon := now.Add(10 * time.Second).UnixMilli()
	f := &fakeKeys{keys: map[string]*clerk.APIKey{
		"ak_ok":       {ID: "1", Subject: owner},
		"ak_expiring": {ID: "2", Subject: owner, Expiration: &soon},
	}}
	v := newKeys(f)
	v.cache.now = func() time.Time { return now }

	for range 3 {
		_, err := v.Verify(t.Context(), "ak_ok")
		require.NoError(t, err)
	}
	assert.Equal(t, 1, f.calls, "successes within the TTL are served from the cache")

	_, err := v.Verify(t.Context(), "ak_expiring")
	require.NoError(t, err)
	now = now.Add(11 * time.Second) // past the key's own expiry, inside the TTL
	_, err = v.Verify(t.Context(), "ak_expiring")
	require.NoError(t, err)
	assert.Equal(t, 3, f.calls, "a cached key must not outlive its own expiration")
}

// --- OAuth tokens ------------------------------------------------------------

type fakeTokens map[string]*verifiedToken

func (f fakeTokens) verify(_ context.Context, token string) (*verifiedToken, error) {
	if t, ok := f[token]; ok {
		return t, nil
	}
	return nil, &clerk.APIErrorResponse{HTTPStatusCode: http.StatusNotFound}
}

func newTokens(f tokenAPI) *OAuthTokens {
	o := options()
	return &OAuthTokens{api: f, resource: resource, tenants: o.Tenants, cache: newCache(o.CacheTTL), log: o.Logger}
}

func TestOAuthTokens(t *testing.T) {
	inactive := false
	v := newTokens(fakeTokens{
		"oat_ok":       {ID: "oat_1", ClientID: "client_claude", Subject: owner, Audience: []string{resource}},
		"oat_other":    {ID: "oat_2", ClientID: "client_claude", Subject: owner, Audience: []string{"https://resumes.example.com/mcp"}},
		"oat_noaud":    {ID: "oat_3", ClientID: "client_claude", Subject: owner},
		"oat_stranger": {ID: "oat_4", ClientID: "client_claude", Subject: "user_stranger", Audience: []string{resource}},
		"oat_revoked":  {ID: "oat_5", Subject: owner, Audience: []string{resource}, Revoked: true},
		"eyJ.a.b":      {Active: &inactive},
	})

	p, err := v.Verify(t.Context(), "oat_ok")
	require.NoError(t, err)
	assert.Equal(t, tenant, p.TenantID)
	assert.Equal(t, "clerk-oauth:client_claude", p.KeyLabel)
	assert.True(t, p.HasScope(domain.ScopeWrite))
	assert.False(t, p.HasScope(domain.ScopeAdmin))

	for _, bad := range []string{"oat_other", "oat_noaud", "oat_stranger", "oat_revoked", "eyJ.a.b", "oat_unknown"} {
		_, err := v.Verify(t.Context(), bad)
		assert.True(t, errors.Is(err, domain.ErrUnauthenticated), "%s: %v", bad, err)
	}
}

func TestIsOAuthToken(t *testing.T) {
	assert.True(t, IsOAuthToken("oat_0ef5a7a33d87ed87ee7954c845d80450"))
	assert.True(t, IsOAuthToken("eyJhbGciOi.eyJzdWIiOi.c2lnbmF0dXJl"))
	assert.False(t, IsOAuthToken("ak_123"))
	assert.False(t, IsOAuthToken("at_live_abc"))
	assert.False(t, IsOAuthToken("eyJonlyonepart"))
}

// TestOAuthVerifyRequestShape runs the real SDK backend against a stand-in for
// Clerk, so the endpoint path, the auth header, and the body field are checked
// against what Clerk's Backend API documents, not against our own fake.
func TestOAuthVerifyRequestShape(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	clerkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"clerk_idp_oauth_access_token","id":"oat_1","client_id":"client_claude",
			"subject":"` + owner + `","scopes":["profile"],"aud":["` + resource + `"],"revoked":false,
			"revocation_reason":null,"expired":false,"expiration":1893456000,"created_at":1,"updated_at":1}`))
	}))
	defer clerkAPI.Close()

	o := options()
	o.SecretKey, o.APIURL = "sk_test_fake", clerkAPI.URL+"/v1"
	p, err := NewOAuthTokens(o, resource).Verify(t.Context(), "oat_0ef5a7a33d87ed87ee7954c845d80450")
	require.NoError(t, err)
	assert.Equal(t, tenant, p.TenantID)
	assert.Equal(t, "/v1/oauth_applications/access_tokens/verify", gotPath)
	assert.Equal(t, "Bearer sk_test_fake", gotAuth)
	assert.Equal(t, "oat_0ef5a7a33d87ed87ee7954c845d80450", gotBody["access_token"])
}

func TestUnixTime(t *testing.T) {
	assert.Equal(t, time.Unix(1893456000, 0), unixTime(1893456000))
	assert.Equal(t, time.UnixMilli(1893456000123), unixTime(1893456000123))
}

func TestFrontendAPI(t *testing.T) {
	// base64("example.clerk.accounts.dev$")
	got, err := FrontendAPI("pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk")
	require.NoError(t, err)
	assert.Equal(t, "https://example.clerk.accounts.dev", got)

	for _, bad := range []string{"", "sk_test_ZXhhbXBsZQ==", "pk_test_!!!", "pk_test_ZXhhbXBsZQ"} {
		_, err := FrontendAPI(bad)
		assert.Error(t, err, bad)
	}
}
