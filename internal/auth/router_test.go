package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

func request(headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestCredential(t *testing.T) {
	cases := map[string]struct {
		headers map[string]string
		want    string
		ok      bool
	}{
		"bearer":            {map[string]string{"Authorization": "Bearer tok"}, "tok", true},
		"lower-case scheme": {map[string]string{"Authorization": "bearer tok"}, "tok", true},
		"x-api-key":         {map[string]string{"X-Api-Key": " tok "}, "tok", true},
		// A stale X-Api-Key must not shadow a fresh OAuth token.
		"authorization wins": {map[string]string{"Authorization": "Bearer oauth", "X-Api-Key": "key"}, "oauth", true},
		"basic scheme":       {map[string]string{"Authorization": "Basic tok"}, "", false},
		"nothing":            {nil, "", false},
	}
	for name, tc := range cases {
		got, ok := auth.Credential(request(tc.headers))
		assert.Equal(t, tc.ok, ok, name)
		assert.Equal(t, tc.want, got, name)
	}

	_, ok := auth.Credential(httptest.NewRequest(http.MethodPost, "/mcp?api_key=tok", nil))
	assert.False(t, ok, "a credential in the URL must never be read")
}

// named answers with its own name as the key label, so a test can see which
// verifier a credential reached.
type named string

func (n named) Verify(context.Context, string) (domain.Principal, error) {
	return domain.Principal{KeyLabel: string(n)}, nil
}

func TestRouterDispatchesByShape(t *testing.T) {
	router := auth.NewRouter(named("own"),
		auth.Route{Name: "clerk key", Match: auth.HasPrefix("ak_"), Verifier: named("clerk key")},
		auth.Route{Name: "clerk oauth", Match: auth.HasPrefix("oat_"), Verifier: named("clerk oauth")},
	)
	for token, want := range map[string]string{
		"ak_123":      "clerk key",
		"oat_abc":     "clerk oauth",
		"at_live_xyz": "own",
		"anything":    "own",
	} {
		p, err := router.Authenticate(t.Context(), request(map[string]string{"Authorization": "Bearer " + token}))
		require.NoError(t, err, token)
		assert.Equal(t, want, p.KeyLabel, token)
	}

	_, err := router.Authenticate(t.Context(), request(nil))
	assert.True(t, errors.Is(err, domain.ErrUnauthenticated))
}
