package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHandler() http.Handler {
	return &handler{csp: contentSecurityPolicy(Options{}), fsys: fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><title>ApplyTrail</title>")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc123.css": {Data: []byte(":root{}")},
		"favicon.svg":             {Data: []byte("<svg/>")},
	}}
}

func get(t *testing.T, path string) *http.Response {
	t.Helper()
	recorder := httptest.NewRecorder()
	testHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Result()
}

func TestServesTheDocumentForClientRoutes(t *testing.T) {
	// Every one of these is a route the browser router owns. The server has
	// never heard of them and must still answer with the document.
	for _, path := range []string{"/", "/pipeline", "/pipeline?app=abc", "/anything/deep"} {
		response := get(t, path)
		require.Equal(t, http.StatusOK, response.StatusCode, path)
		assert.Contains(t, response.Header.Get("Content-Type"), "text/html", path)
		assert.Equal(t, "no-cache", response.Header.Get("Cache-Control"), path)
	}
}

func TestHashedAssetsAreImmutable(t *testing.T) {
	response := get(t, "/assets/index-abc123.js")

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "public, max-age=31536000, immutable", response.Header.Get("Cache-Control"))
	assert.Contains(t, response.Header.Get("Content-Type"), "javascript")
}

func TestMissingAssetIsNotTheDocument(t *testing.T) {
	// Serving index.html here would hand the browser HTML with a JavaScript
	// content type, which surfaces as a syntax error rather than a 404.
	response := get(t, "/assets/index-gone.js")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestNonAssetFilesAreServedButNotCachedForever(t *testing.T) {
	response := get(t, "/favicon.svg")

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "no-cache", response.Header.Get("Cache-Control"))
}

func TestSecurityHeaders(t *testing.T) {
	response := get(t, "/")

	csp := response.Header.Get("Content-Security-Policy")
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src 'self'", "inline script must stay impossible: this page holds a credential")
	assert.Contains(t, csp, "frame-ancestors 'none'")
	assert.Equal(t, "nosniff", response.Header.Get("X-Content-Type-Options"))
	assert.NotContains(t, csp, "https:", "without Clerk, nothing loads from another origin")
}

func TestSecurityHeadersWithClerk(t *testing.T) {
	csp := contentSecurityPolicy(Options{ClerkFrontendAPI: "https://example.clerk.accounts.dev"})

	assert.Contains(t, csp, "script-src 'self' https://example.clerk.accounts.dev https://challenges.cloudflare.com;")
	assert.Contains(t, csp, "connect-src 'self' https://example.clerk.accounts.dev;")
	assert.Contains(t, csp, "frame-src https://challenges.cloudflare.com;")
	assert.NotContains(t, csp, "unsafe-eval")
	assert.NotContains(t, csp, "script-src 'self' 'unsafe-inline'", "Clerk does not need inline script")
}

func TestOnlyReadMethods(t *testing.T) {
	recorder := httptest.NewRecorder()
	testHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	assert.Equal(t, "GET, HEAD", recorder.Header().Get("Allow"))
}

func TestHandlerIsNilWithoutABundle(t *testing.T) {
	// Under the default build tags nothing is embedded, and the router reads a
	// nil handler as "this surface does not exist".
	if Available() {
		t.Skip("built with -tags webui, so a bundle is embedded")
	}
	assert.Nil(t, Handler(Options{}))
}
