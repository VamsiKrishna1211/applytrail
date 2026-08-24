package apikey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/auth/apikey"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

func TestGenerateAndVerify(t *testing.T) {
	key, err := apikey.Generate()
	require.NoError(t, err)

	assert.True(t, len(key.Secret) > 40, "a key must carry real entropy")
	assert.Contains(t, key.Secret, apikey.Prefix, "keys are recognisable in logs and secret scanners")
	assert.NotContains(t, key.Hash, key.Secret, "the stored hash must not contain the key")

	assert.True(t, apikey.Verify(key.Secret, key.Hash))
	assert.False(t, apikey.Verify(key.Secret+"x", key.Hash))

	other, err := apikey.Generate()
	require.NoError(t, err)
	assert.NotEqual(t, key.Secret, other.Secret, "two calls must never collide")
	assert.False(t, apikey.Verify(other.Secret, key.Hash))
}

func TestLookupPrefix(t *testing.T) {
	key, err := apikey.Generate()
	require.NoError(t, err)

	prefix, ok := apikey.LookupPrefix(key.Secret)
	require.True(t, ok)
	assert.Equal(t, key.Prefix, prefix, "verification finds one row by prefix, not by scanning")

	_, ok = apikey.LookupPrefix("not-an-applytrail-key")
	assert.False(t, ok, "garbage is rejected before any database round trip")

	_, ok = apikey.LookupPrefix(apikey.Prefix + "abc")
	assert.False(t, ok, "a truncated key cannot form a prefix")
}

// fakeKeyStore is the minimum store surface the authenticator uses.
type fakeKeyStore struct {
	byPrefix map[string]domain.APIKey
	touched  []domain.ID
}

func (f *fakeKeyStore) GetByPrefix(_ context.Context, prefix string) (domain.APIKey, error) {
	key, ok := f.byPrefix[prefix]
	if !ok {
		return domain.APIKey{}, domain.NewNotFoundError("api key", prefix)
	}
	return key, nil
}

func (f *fakeKeyStore) Touch(_ context.Context, id domain.ID, _ time.Time) error {
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeKeyStore) Create(context.Context, domain.APIKey) (domain.APIKey, error) {
	panic("not used")
}
func (f *fakeKeyStore) List(context.Context, domain.ID) ([]domain.APIKey, error) {
	panic("not used")
}
func (f *fakeKeyStore) Revoke(context.Context, domain.ID, domain.ID) (domain.APIKey, error) {
	panic("not used")
}

func TestAuthenticateAlternateHeader(t *testing.T) {
	generated, err := apikey.Generate()
	require.NoError(t, err)

	record := domain.APIKey{
		ID: domain.NewID(), TenantID: domain.NewID(),
		Prefix: generated.Prefix, Hash: generated.Hash,
		Label: "connector", Scopes: []domain.Scope{domain.ScopeRead},
	}

	other, err := apikey.Generate()
	require.NoError(t, err)

	tests := []struct {
		name    string
		set     func(r *http.Request)
		wantErr bool
		why     string
	}{
		{
			name: "x-api-key carries a bare token",
			set:  func(r *http.Request) { r.Header.Set("X-Api-Key", generated.Secret) },
			why:  "claude.ai connectors reserve Authorization for OAuth and cannot set it",
		},
		{
			name: "header name is case-insensitive",
			set:  func(r *http.Request) { r.Header.Set("x-api-key", generated.Secret) },
			why:  "HTTP header names are case-insensitive and clients vary",
		},
		{
			name: "surrounding whitespace is tolerated",
			set:  func(r *http.Request) { r.Header.Set("X-Api-Key", "  "+generated.Secret+"  ") },
			why:  "a value pasted into a form often carries stray spaces",
		},
		{
			name: "authorization wins when both are present",
			set: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+generated.Secret)
				r.Header.Set("X-Api-Key", other.Secret)
			},
			why: "a stale fallback value must not shadow an explicit Authorization",
		},
		{
			name:    "a bearer scheme in x-api-key is not unwrapped",
			set:     func(r *http.Request) { r.Header.Set("X-Api-Key", "Bearer "+generated.Secret) },
			wantErr: true,
			why:     "the fallback carries a bare token; accepting a scheme would hide a misconfiguration",
		},
		{
			name:    "empty x-api-key is no credential",
			set:     func(r *http.Request) { r.Header.Set("X-Api-Key", "   ") },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := &fakeKeyStore{byPrefix: map[string]domain.APIKey{generated.Prefix: record}}
			authenticator := apikey.New(keys)

			r := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			tc.set(r)

			principal, err := authenticator.Authenticate(t.Context(), r)
			if tc.wantErr {
				require.ErrorIs(t, err, domain.ErrUnauthenticated, tc.why)
				return
			}
			require.NoError(t, err, tc.why)
			assert.Equal(t, record.TenantID, principal.TenantID, tc.why)
			assert.Equal(t, record.ID, principal.KeyID)
		})
	}
}

func TestAuthenticate(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	past := now.AddDate(0, 0, -1)
	future := now.AddDate(0, 0, 1)

	generated, err := apikey.Generate()
	require.NoError(t, err)
	tenantID := domain.NewID()
	keyID := domain.NewID()

	base := domain.APIKey{
		ID: keyID, TenantID: tenantID, Prefix: generated.Prefix, Hash: generated.Hash,
		Label: "search-agent", Scopes: []domain.Scope{domain.ScopeRead, domain.ScopeWrite},
	}

	tests := []struct {
		name    string
		header  string
		record  *domain.APIKey
		wantErr bool
		why     string
	}{
		{name: "valid key", header: "Bearer " + generated.Secret, record: &base},
		{
			name: "lowercase scheme", header: "bearer " + generated.Secret, record: &base,
			why: "clients vary in casing and rejecting them helps nobody",
		},
		{name: "no header", header: "", record: &base, wantErr: true},
		{name: "wrong scheme", header: "Basic " + generated.Secret, record: &base, wantErr: true},
		{name: "unknown key", header: "Bearer " + apikey.Prefix + "deadbeefdeadbeef", record: &base, wantErr: true},
		{
			name:    "revoked key",
			header:  "Bearer " + generated.Secret,
			record:  func() *domain.APIKey { k := base; k.RevokedAt = &past; return &k }(),
			wantErr: true,
		},
		{
			name:    "expired key",
			header:  "Bearer " + generated.Secret,
			record:  func() *domain.APIKey { k := base; k.ExpiresAt = &past; return &k }(),
			wantErr: true,
		},
		{
			name:   "key expiring later is still valid",
			header: "Bearer " + generated.Secret,
			record: func() *domain.APIKey { k := base; k.ExpiresAt = &future; return &k }(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := &fakeKeyStore{byPrefix: map[string]domain.APIKey{generated.Prefix: *tc.record}}
			authenticator := apikey.New(keys).WithClock(func() time.Time { return now })

			r := httptest.NewRequest(http.MethodGet, "/v1/applications", nil)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}

			principal, err := authenticator.Authenticate(t.Context(), r)
			if tc.wantErr {
				require.ErrorIs(t, err, domain.ErrUnauthenticated)
				assert.Equal(t, "unauthenticated: missing or invalid API key", err.Error(),
					"every failure must look identical from outside, or the error tells an attacker which keys exist")
				return
			}
			require.NoError(t, err, tc.why)
			assert.Equal(t, tenantID, principal.TenantID)
			assert.Equal(t, keyID, principal.KeyID)
			assert.True(t, principal.HasScope(domain.ScopeWrite))
			assert.Contains(t, keys.touched, keyID, "successful use is recorded for the audit trail")
		})
	}
}
