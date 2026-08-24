// Package auth authenticates callers and carries the result on the context.
//
// The Authenticator port exists so a deployment can add browser login without
// touching a single handler: the HTTP layer only ever asks "who is this?" and
// receives a domain.Principal.
package auth

import (
	"context"
	"net/http"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Authenticator resolves a request to a principal.
//
// Implementations return an error wrapping domain.ErrUnauthenticated when the
// credential is missing, malformed, expired, or revoked. They must not
// distinguish between those cases in the message they expose to the client,
// because doing so tells an attacker which keys exist.
type Authenticator interface {
	Authenticate(ctx context.Context, r *http.Request) (domain.Principal, error)
}

type principalKey struct{}

// ContextWithPrincipal returns a context carrying p.
func ContextWithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext returns the authenticated caller.
//
// The boolean is false on an unauthenticated context. Service methods take a
// principal as an explicit argument rather than reading it from the context,
// so that a caller cannot forget to authorise; this helper exists for the
// transport layer that puts it there.
func PrincipalFromContext(ctx context.Context) (domain.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(domain.Principal)
	return p, ok
}
