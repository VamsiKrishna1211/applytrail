package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
)

// stubUI stands in for the embedded bundle so this test needs neither a
// database nor a UI build.
var stubUI = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html>"))
})

func routerWithUI(ui http.Handler) http.Handler {
	return httpapi.NewRouter(httpapi.Deps{Version: "test", WebUI: ui})
}

func requestTo(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestUnmatchedBrowserPathsReachTheUI(t *testing.T) {
	router := routerWithUI(stubUI)

	for _, path := range []string{"/", "/pipeline", "/assets/index-abc.js"} {
		recorder := requestTo(t, router, path)
		require.Equal(t, http.StatusOK, recorder.Code, path)
		assert.Contains(t, recorder.Body.String(), "<!doctype html>", path)
	}
}

func TestAPIMissesStayJSONEvenWithAUI(t *testing.T) {
	router := routerWithUI(stubUI)

	// A client calling a path that does not exist must be told so in the
	// envelope it knows how to parse. Handing it the browser document would
	// turn a routing mistake into a JSON parse error several layers away.
	for _, path := range []string{"/v1/nope", "/v1/applications/x/y", "/mcp/nope"} {
		recorder := requestTo(t, router, path)
		require.Equal(t, http.StatusNotFound, recorder.Code, path)
		assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json", path)
		assert.Contains(t, recorder.Body.String(), `"not_found"`, path)
	}
}

func TestWithoutAUIEverythingIsStillJSON(t *testing.T) {
	router := routerWithUI(nil)

	recorder := requestTo(t, router, "/pipeline")

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
}

func TestHealthProbesAreNotShadowedByTheUI(t *testing.T) {
	router := routerWithUI(stubUI)

	recorder := requestTo(t, router, "/readyz")

	// /readyz answers 503 with no database configured, which is the point:
	// it reached the probe rather than the browser document.
	assert.NotContains(t, recorder.Body.String(), "<!doctype html>")
}
