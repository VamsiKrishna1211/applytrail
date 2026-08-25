package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/ratelimit"
	"github.com/VamsiKrishna1211/applytrail/internal/service"
)

// Deps are the collaborators the router needs. Fields are added as milestones
// land; a nil dependency disables the routes that require it, which keeps the
// server runnable at every stage of the build.
type Deps struct {
	Version string

	// DB backs the readiness probe.
	DB Pinger

	// Service is the use-case layer. Handlers do nothing but translate for it.
	Service *service.Service

	// Authenticator resolves callers. The port, not the API key
	// implementation, so a deployment can add browser login without touching
	// a handler.
	Authenticator auth.Authenticator

	// Limiter bounds request rate per credential. Nil disables limiting.
	Limiter ratelimit.Limiter

	// AuthFailureLimiter bounds how fast credentials can be guessed. It is
	// separate from Limiter because Limiter keys on the authenticated
	// principal, which a rejected request does not have. Nil disables it.
	AuthFailureLimiter ratelimit.Limiter

	// MCPHandler serves the Model Context Protocol endpoint. It is a plain
	// net/http handler from the MCP SDK and mounts directly at /mcp.
	MCPHandler http.Handler

	// WebUI serves the browser client from the embedded bundle. Nil means the
	// binary was built without one, and every unmatched path stays a JSON 404
	// exactly as it was before the UI existed.
	//
	// It is deliberately unauthenticated: it serves a static document and
	// nothing else. The credential is supplied by the person using it and
	// spent on /v1, which authenticates as it always has.
	WebUI http.Handler
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

	// The MCP endpoint authenticates and rate-limits exactly like the REST
	// API: the transport differs, the trust boundary does not.
	if deps.MCPHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(otelMiddleware("mcp"))
			r.Use(authenticate(deps.Authenticator, deps.AuthFailureLimiter))
			if deps.Limiter != nil {
				r.Use(rateLimit(deps.Limiter))
			}
			r.Handle("/mcp", deps.MCPHandler)
			r.Handle("/mcp/*", deps.MCPHandler)
		})
	}

	if deps.Service != nil {
		h := &handlers{svc: deps.Service}
		r.Route("/v1", func(r chi.Router) {
			r.Use(otelMiddleware("v1"))
			r.Use(authenticate(deps.Authenticator, deps.AuthFailureLimiter))
			if deps.Limiter != nil {
				r.Use(rateLimit(deps.Limiter))
			}

			// Batch dedupe is read-only on purpose, so a search agent can run
			// with a credential that cannot mutate anything.
			r.With(requireScope(domain.ScopeRead)).Post("/postings:filter", h.handleFilterPostings)
			r.With(requireScope(domain.ScopeWrite)).Post("/postings:ingest", h.handleIngestPostings)

			r.Route("/applications", func(r chi.Router) {
				r.With(requireScope(domain.ScopeRead)).Get("/", h.handleListApplications)
				r.With(requireScope(domain.ScopeRead)).Get("/followups", h.handleFollowups)
				r.With(requireScope(domain.ScopeRead)).Get("/{id}", h.handleGetApplication)
				r.With(requireScope(domain.ScopeWrite)).Post("/", h.handleRecordApplication)
				r.With(requireScope(domain.ScopeWrite)).Patch("/{id}", h.handleUpdateApplication)
				r.With(requireScope(domain.ScopeWrite)).Post("/{id}/notes", h.handleAddNote)
			})

			r.Route("/search-profiles", func(r chi.Router) {
				r.With(requireScope(domain.ScopeRead)).Get("/", h.handleListProfiles)
				r.With(requireScope(domain.ScopeRead)).Get("/{name}", h.handleGetProfile)
				r.With(requireScope(domain.ScopeWrite)).Post("/", h.handleCreateProfile)
				r.With(requireScope(domain.ScopeWrite)).Patch("/{id}", h.handleUpdateProfile)
			})

			r.Route("/search-runs", func(r chi.Router) {
				r.With(requireScope(domain.ScopeRead)).Get("/", h.handleListRuns)
				r.With(requireScope(domain.ScopeWrite)).Post("/", h.handleStartRun)
				r.With(requireScope(domain.ScopeWrite)).Patch("/{id}", h.handleFinishRun)
			})

			r.Route("/contacts", func(r chi.Router) {
				r.With(requireScope(domain.ScopeRead)).Get("/", h.handleListContacts)
				r.With(requireScope(domain.ScopeWrite)).Post("/", h.handleUpsertContact)
			})

			r.With(requireScope(domain.ScopeRead)).Get("/stats", h.handleStats)

			r.Route("/admin/api-keys", func(r chi.Router) {
				r.Use(requireScope(domain.ScopeAdmin))
				r.Get("/", h.handleListKeys)
				r.Post("/", h.handleCreateKey)
				r.Delete("/{id}", h.handleRevokeKey)
			})
		})
	}

	r.NotFound(notFound(deps.WebUI))
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusMethodNotAllowed, errorBody{Error: errorDetail{
			Code:    "method_not_allowed",
			Message: r.Method + " is not allowed on " + r.URL.Path,
		}})
	})

	return r
}

// apiPrefixes are the paths that belong to a machine surface. A miss under one
// of them is a routing mistake and must read as one, in the same JSON envelope
// as every other API error — never as the browser client's index document.
var apiPrefixes = []string{"/v1", "/mcp", "/healthz", "/readyz", "/version"}

// notFound answers an unmatched path.
//
// With no UI compiled in, or on any API path, it is the JSON envelope. With a
// UI, every other path is handed to the client router, because a deep link
// like /pipeline is a real page that only the browser knows how to resolve.
func notFound(ui http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ui != nil && !isAPIPath(r.URL.Path) {
			ui.ServeHTTP(w, r)
			return
		}
		writeJSON(w, r, http.StatusNotFound, errorBody{Error: errorDetail{
			Code:    "not_found",
			Message: "no route matches " + r.Method + " " + r.URL.Path,
		}})
	}
}

func isAPIPath(path string) bool {
	for _, prefix := range apiPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// otelMiddleware instruments a route group and names its spans after the
// matched chi pattern.
func otelMiddleware(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(nameSpanFromRoute(next), operation)
	}
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
