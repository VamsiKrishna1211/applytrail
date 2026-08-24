package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Deps are the collaborators the router needs. Fields are added as milestones
// land; a nil dependency disables the routes that require it, which keeps the
// server runnable at every stage of the build.
type Deps struct {
	Version string

	// DB backs the readiness probe.
	DB Pinger

	// MCPHandler serves the Model Context Protocol endpoint. It is a plain
	// net/http handler from the MCP SDK and mounts directly at /mcp.
	MCPHandler http.Handler
}

// NewRouter assembles the HTTP surface: health probes, the versioned REST API,
// and the MCP endpoint.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(requestID)
	r.Use(recoverer)
	r.Use(requestLogger)
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)

	// Health probes sit outside authentication and instrumentation so a
	// platform can always reach them.
	r.Get("/healthz", handleLive(deps.Version))
	r.Get("/readyz", handleReady(deps.DB))
	r.Get("/version", handleLive(deps.Version))

	if deps.MCPHandler != nil {
		r.Handle("/mcp", otelhttp.NewHandler(deps.MCPHandler, "mcp"))
		r.Handle("/mcp/*", otelhttp.NewHandler(deps.MCPHandler, "mcp"))
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(nameSpanFromRoute(next), "v1")
		})
		// REST routes are registered here as milestone M3 lands.
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusNotFound, errorBody{Error: errorDetail{
			Code:    "not_found",
			Message: "no route matches " + r.Method + " " + r.URL.Path,
		}})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusMethodNotAllowed, errorBody{Error: errorDetail{
			Code:    "method_not_allowed",
			Message: r.Method + " is not allowed on " + r.URL.Path,
		}})
	})

	return r
}

// nameSpanFromRoute renames the active span to chi's matched route pattern,
// so /v1/applications/{id} is one span name instead of one per application.
//
// The rename happens after next returns because chi only knows the pattern
// once routing has resolved, and otelhttp keeps the span open until the whole
// chain unwinds.
func nameSpanFromRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rctx := chi.RouteContext(r.Context())
			if rctx == nil {
				return
			}
			pattern := rctx.RoutePattern()
			if pattern == "" {
				return
			}
			span := trace.SpanFromContext(r.Context())
			span.SetName(r.Method + " " + pattern)
			span.SetAttributes(semconv.HTTPRoute(pattern))
		}()
		next.ServeHTTP(w, r)
	})
}
