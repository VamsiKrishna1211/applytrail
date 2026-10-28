package httpapi

import "net/http"

// webConfigPath is where the browser client learns how to sign in. It sits
// outside /v1 because it is read before the client holds any credential.
const webConfigPath = "/auth/config"

// webConfig is what the browser client needs before it can sign in.
type webConfig struct {
	// ClerkPublishableKey is set when the UI should offer Clerk sign-in. It
	// is public by design; Clerk embeds it in every page that uses it.
	ClerkPublishableKey string `json:"clerk_publishable_key,omitempty"`
}

// handleWebConfig serves the sign-in configuration. It is read at runtime
// rather than compiled into the bundle, so one image serves any deployment.
func handleWebConfig(publishableKey string) http.HandlerFunc {
	body := webConfig{ClerkPublishableKey: publishableKey}
	return func(w http.ResponseWriter, r *http.Request) {
		// A redeploy can switch Clerk on or off; a cached answer would leave
		// the UI offering a sign-in the server no longer accepts.
		w.Header().Set("Cache-Control", "no-cache")
		writeJSON(w, r, http.StatusOK, body)
	}
}
