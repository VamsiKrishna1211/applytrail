package main

import (
	"fmt"
	"log/slog"

	"github.com/VamsiKrishna1211/applytrail/internal/auth"
	"github.com/VamsiKrishna1211/applytrail/internal/auth/apikey"
	"github.com/VamsiKrishna1211/applytrail/internal/auth/clerkauth"
	"github.com/VamsiKrishna1211/applytrail/internal/config"
	"github.com/VamsiKrishna1211/applytrail/internal/store"
	"github.com/VamsiKrishna1211/applytrail/internal/transport/httpapi"
)

// authSetup is everything authentication decides: who is let in, and what
// the public surfaces tell clients about signing in.
type authSetup struct {
	Authenticator auth.Authenticator
	// ProtectedResource is nil while OAuth is off.
	ProtectedResource *httpapi.ProtectedResource
	// ClerkPublishableKey and ClerkFrontendAPI are empty while the web UI
	// does not sign in with Clerk.
	ClerkPublishableKey string
	ClerkFrontendAPI    string
}

// authentication chooses how callers prove who they are.
//
// ApplyTrail's own keys always work, and are the only credential that can
// hold admin. With Clerk configured, Clerk API keys are added. With its issuer
// configured too, so are Clerk OAuth tokens, along with the discovery metadata
// that lets an MCP client find Clerk and sign the user in. With its
// publishable key configured, so are the session tokens the web UI gets when
// a person signs in to it with Clerk.
func authentication(cfg config.ClerkConfig, keys store.APIKeyStore) (authSetup, error) {
	own := apikey.New(keys)
	if !cfg.Enabled() {
		return authSetup{Authenticator: own}, nil
	}

	tenants, err := clerkauth.ParseTenants(cfg.Tenants)
	if err != nil {
		return authSetup{}, fmt.Errorf("APPLYTRAIL_CLERK_TENANTS: %w", err)
	}
	opts := clerkauth.Options{SecretKey: cfg.SecretKey, APIURL: cfg.APIURL, Tenants: tenants}

	setup := authSetup{}
	routes := []auth.Route{{
		Name:     "clerk api key",
		Match:    auth.HasPrefix(clerkauth.APIKeyPrefix),
		Verifier: clerkauth.NewAPIKeys(opts),
	}}
	// Session tokens come before OAuth tokens: both can be JWTs, and only
	// the session matcher looks inside.
	if cfg.WebSignInEnabled() {
		fapi, err := clerkauth.FrontendAPI(cfg.PublishableKey)
		if err != nil {
			return authSetup{}, fmt.Errorf("APPLYTRAIL_CLERK_PUBLISHABLE_KEY: %w", err)
		}
		routes = append(routes, auth.Route{
			Name:     "clerk session token",
			Match:    clerkauth.IsSessionToken,
			Verifier: clerkauth.NewSessionTokens(opts, cfg.AuthorizedParties()),
		})
		setup.ClerkPublishableKey, setup.ClerkFrontendAPI = cfg.PublishableKey, fapi
	}
	if cfg.OAuthEnabled() {
		routes = append(routes, auth.Route{
			Name:     "clerk oauth token",
			Match:    clerkauth.IsOAuthToken,
			Verifier: clerkauth.NewOAuthTokens(opts, cfg.MCPResource()),
		})
		setup.ProtectedResource = &httpapi.ProtectedResource{Resource: cfg.MCPResource(), AuthorizationServer: cfg.Issuer}
	}
	slog.Info("clerk authentication enabled",
		"oauth", cfg.OAuthEnabled(), "web_sign_in", cfg.WebSignInEnabled(),
		"subjects", len(tenants), "resource", cfg.MCPResource(), "web_origins", cfg.AuthorizedParties())
	setup.Authenticator = auth.NewRouter(own, routes...)
	return setup, nil
}
