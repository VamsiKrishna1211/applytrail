package auth

import (
	"net/http"
	"strings"
)

// APIKeyHeader carries the credential when Authorization is unavailable.
//
// Some MCP clients reserve Authorization for their own OAuth token and will not
// let a user set it; claude.ai's custom connector is one. They offer a short
// list of alternative header names instead, of which this is the most common.
const APIKeyHeader = "X-Api-Key" //nolint:gosec // a header name, not a credential

// Credential extracts the credential from the request.
//
// Authorization is the primary channel and is tried first. X-Api-Key is a
// fallback carrying the bare token with no scheme, for clients that cannot set
// Authorization. Presenting both is not an error: Authorization wins, so a
// client that sets one of them cannot be confused by a stale value in the
// other. Every scheme reads the credential here, so they cannot disagree about
// which one a request carries.
func Credential(r *http.Request) (string, bool) {
	if token, ok := bearerToken(r); ok {
		return token, true
	}
	token := strings.TrimSpace(r.Header.Get(APIKeyHeader))
	if token == "" {
		return "", false
	}
	return token, true
}

// bearerToken extracts the credential from the Authorization header.
//
// The scheme match is case-insensitive because clients send "Bearer", "bearer",
// and occasionally "BEARER", and rejecting those would be a confusing failure
// for no benefit.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}
