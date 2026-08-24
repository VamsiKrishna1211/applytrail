// Package oidc is a placeholder for browser-based login.
//
// ApplyTrail authenticates agents with API keys, which is the right fit for a
// headless caller: no redirect flow, no token refresh, no browser. A hosted
// deployment with a human-facing UI needs something else, and this package
// marks where it goes.
//
// Implementing it means satisfying one interface:
//
//	type Authenticator interface {
//	    Authenticate(ctx context.Context, r *http.Request) (domain.Principal, error)
//	}
//
// Validate the ID token, map its subject to a tenant, and return a Principal
// with the scopes that user should hold. Nothing in internal/service or
// internal/transport needs to change: they already depend on the port rather
// than on API keys. Wire the implementation in cmd/applytrail-server, or
// compose both so keys and sessions work side by side.
package oidc
