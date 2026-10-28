package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

// ProtectedResource describes this server to OAuth clients, per RFC 9728.
// Claude reads it to find the authorization server, then signs the user in
// there. Nil Deps.ProtectedResource means OAuth is off and nothing is served.
type ProtectedResource struct {
	// Resource is the MCP URL exactly as a user types it into a client. It
	// must equal the audience the authorization server puts in its tokens.
	Resource string
	// AuthorizationServer is the issuer URL. Claude uses only the first
	// entry of the list, so there is exactly one.
	AuthorizationServer string
}

// MetadataURL is where the document is served: the well-known prefix, then
// the resource's own path, as RFC 9728 section 3.1 lays out.
func (p ProtectedResource) MetadataURL() string {
	u, err := url.Parse(p.Resource)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + metadataPathPrefix + strings.TrimSuffix(u.Path, "/")
}

const metadataPathPrefix = "/.well-known/oauth-protected-resource"

// protectedResourceDocument is the RFC 9728 metadata body.
type protectedResourceDocument struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name"`
}

// handleProtectedResource serves the metadata. It is public — a client reads
// it before it holds any credential — and open to any origin, because a
// browser-based MCP client fetches it cross-origin.
//
// The scopes are Clerk's standard ones. Claude requests whatever is listed
// here, and an OAuth token is not narrowed by scope in ApplyTrail anyway:
// signing in grants read and write to the signed-in user's tenant.
func handleProtectedResource(p ProtectedResource) http.HandlerFunc {
	doc := protectedResourceDocument{
		Resource:               p.Resource,
		AuthorizationServers:   []string{p.AuthorizationServer},
		ScopesSupported:        []string{"profile", "email"},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "ApplyTrail",
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		writeJSON(w, r, http.StatusOK, doc)
	}
}

// challenge is the WWW-Authenticate value on a 401.
//
// With OAuth on, the Bearer challenge carries resource_metadata, which is what
// starts sign-in in Claude and other spec-following MCP clients. With it off,
// the api_key challenge is what stops such a client reading a bare Bearer
// challenge as an invitation to start an OAuth flow this server does not
// implement. Both list api_key, since keys keep working either way.
func challenge(p *ProtectedResource) string {
	if p == nil {
		return `Bearer realm="applytrail", api_key realm="applytrail"`
	}
	return `Bearer resource_metadata="` + p.MetadataURL() + `", api_key realm="applytrail"`
}
