/**
 * A signed-in identity that mints its own short-lived tokens.
 *
 * Clerk, today. Unlike an API key, nothing here is held by the app: each
 * request asks for a token, and the identity refreshes it as it expires.
 */
export interface IdentityProvider {
  /** A token for the next request, or undefined while nobody is signed in. */
  token(): Promise<string | undefined>
  /** Ends the session, e.g. after the server refused it. */
  signOut(): Promise<void>
}
