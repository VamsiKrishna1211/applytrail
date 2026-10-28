package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
func authenticate(authenticator auth.Authenticator, failures ratelimit.Limiter, wwwAuthenticate string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, err := authenticator.Authenticate(r.Context(), r)
			if err != nil {
				// The budget exists to slow down credential guessing, so it
				// must only be spent on a genuine bad-credential rejection
				// (wraps ErrUnauthenticated). Authenticate can also fail with
				// a plain backend error — a database outage, say — and that
				// must reach the caller as the 500 it is, not be converted
				// into a 429 by a check that thinks it is one more guess.
				if errors.Is(err, domain.ErrUnauthenticated) {
					if retryAfter, ok := authFailureExhausted(r, failures); ok {
						w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
						writeError(w, r, fmt.Errorf("%w: too many failed authentication attempts", domain.ErrRateLimited))
						return
					}
				}
				// The identity provider did not answer. The credential may be
				// fine, so this is a retry, not a challenge to re-authenticate.
				if errors.Is(err, domain.ErrUnavailable) {
					w.Header().Set("Retry-After", "5")
					writeError(w, r, err)
					return
				}
				// Advertise the accepted schemes so a client knows what to
				// send; see challenge for why the value differs with OAuth on.
				w.Header().Set("WWW-Authenticate", wwwAuthenticate)
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

// authFailureBudgetKey is a single shared bucket rather than one per client.
//
// Keying on the client address would look tighter but is not: the address
// arrives in X-Forwarded-For, which the caller controls, so an attacker rotates
// it for free while a legitimate agent behind a shared egress gets throttled
// for someone else's mistakes. One global budget cannot be evaded that way, and
// it only ever governs requests that already failed to authenticate — a caller
// holding a valid key never touches it.
const authFailureBudgetKey = "auth-failure"

// authFailureExhausted consumes one unit of the failure budget and reports
// whether the caller has run out, along with how long to wait.
//
// A limiter error is deliberately treated as "not exhausted": a broken limiter
// must not convert a 401 into a 429 and mislead a client about why it was
// rejected.
func authFailureExhausted(r *http.Request, failures ratelimit.Limiter) (time.Duration, bool) {
	if failures == nil {
		return 0, false
	}
	decision, err := failures.Allow(r.Context(), authFailureBudgetKey)
	if err != nil {
		slog.ErrorContext(r.Context(), "auth failure limiter failed open", "error", err)
		return 0, false
	}
	if decision.Allowed {
		return 0, false
	}
	return decision.RetryAfter, true
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
				// Every mount point puts authenticate() ahead of this
				// middleware, so a missing principal here should never
				// happen. Failing closed rather than passing the request
				// through unlimited is what keeps that true even if a future
				// route is wired in the wrong order.
				writeError(w, r, fmt.Errorf("%w: no credential on request", domain.ErrUnauthenticated))
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
