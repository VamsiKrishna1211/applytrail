package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
)

// rejectAll fails every request with err, so the challenge and status a
// client sees can be checked with no database.
type rejectAll struct{ err error }

func (r rejectAll) Authenticate(context.Context, *http.Request) (domain.Principal, error) {
	return domain.Principal{}, r.err
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

var resource = &httpapi.ProtectedResource{
	Resource:            "https://applytrail.example.com/mcp",
	AuthorizationServer: "https://example.clerk.accounts.dev",
}

func TestProtectedResourceMetadata(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Deps{Version: "test", ProtectedResource: resource})

	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

		var doc map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
		assert.Equal(t, resource.Resource, doc["resource"], path)
		assert.Equal(t, []any{resource.AuthorizationServer}, doc["authorization_servers"], path)
	}
	assert.Equal(t, "https://applytrail.example.com/.well-known/oauth-protected-resource/mcp", resource.MetadataURL())
}

func TestNoMetadataWithoutOAuth(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Deps{Version: "test"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestChallengePointsAtMetadataWhenOAuthIsOn(t *testing.T) {
	unauthenticated := rejectAll{err: fmt.Errorf("%w: missing or invalid API key", domain.ErrUnauthenticated)}
	cases := map[string]struct {
		resource *httpapi.ProtectedResource
		want     string
	}{
		"oauth on":  {resource, `Bearer resource_metadata="https://applytrail.example.com/.well-known/oauth-protected-resource/mcp", api_key realm="applytrail"`},
		"oauth off": {nil, `Bearer realm="applytrail", api_key realm="applytrail"`},
	}
	for name, tc := range cases {
		router := httpapi.NewRouter(httpapi.Deps{
			Version: "test", Authenticator: unauthenticated, MCPHandler: okHandler, ProtectedResource: tc.resource,
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.Equal(t, tc.want, rec.Header().Get("WWW-Authenticate"), name)
	}
}

func TestIdentityProviderOutageIsARetry(t *testing.T) {
	router := httpapi.NewRouter(httpapi.Deps{
		Version:           "test",
		Authenticator:     rejectAll{err: fmt.Errorf("%w: clerk did not answer", domain.ErrUnavailable)},
		MCPHandler:        okHandler,
		ProtectedResource: resource,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "5", rec.Header().Get("Retry-After"))
	assert.Empty(t, rec.Header().Get("WWW-Authenticate"), "an outage must not ask the client to sign in again")
	assert.NotContains(t, rec.Body.String(), "clerk did not answer", "the cause stays in the log")
}
