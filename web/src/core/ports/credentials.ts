/**
 * Where the API key lives.
 *
 * A port rather than a direct sessionStorage call, because this is exactly the
 * decision most likely to change: a deployment that grows cookie-based browser
 * login replaces the adapter and nothing else.
 */
export interface CredentialStore {
  read(): string | undefined
  write(token: string): void
  clear(): void
  /** Notifies on any change, including one made in another tab. Returns an unsubscribe. */
  subscribe(listener: () => void): () => void
}
