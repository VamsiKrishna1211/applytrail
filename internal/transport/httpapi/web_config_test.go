package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
)

func TestWebConfig(t *testing.T) {
	cases := map[string]struct {
		key  string
		body string
	}{
		"clerk on":  {key: "pk_test_abc", body: `{"clerk_publishable_key":"pk_test_abc"}`},
		"clerk off": {body: `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Public: the browser reads it before it has signed in, so the
			// router needs no authenticator to serve it.
			router := httpapi.NewRouter(httpapi.Deps{Version: "test", ClerkPublishableKey: tc.key})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/config", nil))

			require.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, tc.body, rec.Body.String())
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
		})
	}
}

func TestWebConfigIsNeverTheUIDocument(t *testing.T) {
	// A miss under /auth is an API miss, not a client route.
	router := httpapi.NewRouter(httpapi.Deps{Version: "test", WebUI: okHandler})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/nope", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
