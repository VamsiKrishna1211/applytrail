package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/VamsiKrishna1211/applytrail/internal/domain"
)

// Verifier resolves one raw credential to a principal. Each credential scheme
// is a Verifier; Router decides which one a request's credential belongs to.
type Verifier interface {
	Verify(ctx context.Context, token string) (domain.Principal, error)
}

// Route sends credentials that Match to Verifier.
type Route struct {
	// Name labels the route in errors and logs.
	Name     string
	Match    func(token string) bool
	Verifier Verifier
}

// HasPrefix matches credentials starting with prefix.
func HasPrefix(prefix string) func(string) bool {
	return func(token string) bool { return strings.HasPrefix(token, prefix) }
}

// Router is an Authenticator that reads the request's credential once and
// dispatches it by shape to the first matching route, or to the fallback.
//
// Dispatching by shape, rather than offering the credential to each verifier
// in turn, means a credential is never sent to a service it cannot belong to:
// an ApplyTrail key never reaches Clerk, and a Clerk token never reaches the
// database.
type Router struct {
	routes   []Route
	fallback Verifier
}

var _ Authenticator = (*Router)(nil)

// NewRouter returns a Router. Credentials matching no route go to fallback.
func NewRouter(fallback Verifier, routes ...Route) *Router {
	return &Router{routes: routes, fallback: fallback}
}

// Authenticate implements Authenticator.
func (rt *Router) Authenticate(ctx context.Context, r *http.Request) (domain.Principal, error) {
	token, ok := Credential(r)
	if !ok {
		return domain.Principal{}, fmt.Errorf("%w: missing or invalid API key", domain.ErrUnauthenticated)
	}
	for _, route := range rt.routes {
		if route.Match(token) {
			return route.Verifier.Verify(ctx, token)
		}
	}
	return rt.fallback.Verify(ctx, token)
}
