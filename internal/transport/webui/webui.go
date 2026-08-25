// Package webui serves the single-page browser client.
//
// The bundle is compiled into the binary, so a deployment is still one
// artifact: no second host, no static bucket, and no CORS configuration
// anywhere, because the UI and the API answer on the same origin.
//
// The assets are only embedded under the `webui` build tag. Without it this
// package still compiles and Available reports false, which is what lets
// `go build ./...` and `go test ./...` work on a checkout that has never run
// npm.
package webui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// assetPrefix is where Vite writes content-hashed files. Everything under it
// is immutable by construction: a changed file gets a changed name.
const assetPrefix = "assets/"

// indexFile is served for any path that is not a file, because the client
// router owns those paths.
const indexFile = "index.html"

// Available reports whether a UI bundle was compiled into this binary.
func Available() bool {
	_, ok := assets()
	return ok
}

// Handler serves the embedded bundle, or nil when none was compiled in.
//
// A nil return is deliberate rather than a handler that reports the UI is
// missing: the router treats a nil dependency as "this surface does not
// exist", so a server built without the tag answers exactly as it did before
// the UI existed.
func Handler() http.Handler {
	fsys, ok := assets()
	if !ok {
		return nil
	}
	return &handler{fsys: fsys}
}

type handler struct {
	fsys fs.FS
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		// The UI is static. Anything else is a client bug, and answering 404
		// would hide it.
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

	if name != "" && name != indexFile {
		if h.serveFile(w, r, name) {
			return
		}
		// A hashed asset that is not there is a genuine miss. Falling back to
		// the document would return HTML with a JavaScript content type and
		// produce a syntax error in the console instead of a 404 in the
		// network tab.
		if strings.HasPrefix(name, assetPrefix) {
			http.NotFound(w, r)
			return
		}
	}

	// Every other path belongs to the client router: deep links, unknown
	// routes, and the root.
	if !h.serveFile(w, r, indexFile) {
		http.Error(w, "web UI bundle is incomplete", http.StatusInternalServerError)
	}
}

// serveFile writes name from the bundle, reporting whether it existed.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	file, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return false
	}

	setSecurityHeaders(w)
	if strings.HasPrefix(name, assetPrefix) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// The document names the hashed assets, so it is the one file that
		// must never be served from a stale cache after a deploy.
		w.Header().Set("Cache-Control", "no-cache")
	}

	// ServeContent negotiates range requests and conditional GETs, and infers
	// the content type from the extension.
	http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
	return true
}

// setSecurityHeaders locks the document down.
//
// This matters more here than on a typical static site: the page holds an API
// key for the life of the tab, so the budget for injected script is zero.
// Inline styles stay allowed because the UI sets widths and colours from data;
// inline script does not.
func setSecurityHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'none'",
	}, "; "))
}
