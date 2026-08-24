package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/domain"
	"github.com/VamsiKrishna1211/applytrail/internal/observability"
	"github.com/VamsiKrishna1211/applytrail/internal/ratelimit"
)

// authenticate resolves the caller and publishes the principal on the context.
//
// It also adds the key's identity to the log fields, so every later line in
// the request carries which credential made it: that is the audit trail when
// several agents share one deployment.
func authenticate(authenticator auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, err := authenticator.Authenticate(r.Context(), r)
			if err != nil {
				// Advertise the scheme so a client knows what to send.
				w.Header().Set("WWW-Authenticate", `Bearer realm="applytrail"`)
				writeError(w, r, err)
				return
			}

			ctx := auth.ContextWithPrincipal(r.Context(), principal)
			ctx = observability.ContextWithLogFields(ctx,
				slog.String("key_label", principal.KeyLabel),
				slog.String("tenant_id", principal.TenantID.String()),
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// rateLimit bounds request rate per credential.
//
// Keying on the API key rather than on the client IP is deliberate: an agent
// running in a cloud sandbox shares its address with everyone else there, so
// an IP-keyed limit would throttle unrelated callers.
func rateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				// Unauthenticated requests never reach here; if one does, the
				// safe response is to let the auth middleware reject it.
				next.ServeHTTP(w, r)
				return
			}

			decision, err := limiter.Allow(r.Context(), principal.KeyID.String())
			if err != nil {
				// A limiter failure must not take the API down with it.
				slog.ErrorContext(r.Context(), "rate limiter failed open", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !decision.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(decision.RetryAfter.Seconds())))
				writeError(w, r, fmt.Errorf("%w: retry in %s", domain.ErrRateLimited, decision.RetryAfter))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireScope rejects a caller whose credential lacks the needed permission.
func requireScope(scope domain.Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				writeError(w, r, fmt.Errorf("%w: no credential on request", domain.ErrUnauthenticated))
				return
			}
			if err := principal.RequireScope(scope); err != nil {
				writeError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// principalOf reads the authenticated caller, which the auth middleware
// guarantees is present on every route it guards.
func principalOf(r *http.Request) domain.Principal {
	principal, _ := auth.PrincipalFromContext(r.Context())
	return principal
}
