/**
 * Why a credential was rejected.
 *
 * The transport drops a credential the server refuses — leaving it in place
 * would mean a signed-in UI where every request fails. But dropping it without
 * saying so is worse: the unlock screen reappears with no explanation, and a
 * typo, an expired key and a key belonging to a different database all look
 * identical.
 *
 * This port carries the reason from the transport to the unlock screen.
 */
export interface AuthRejection {
  /** The server's error code, or `network` when it never answered. */
  code: string
  message: string
  /** Matches X-Request-Id and the server's logs. */
  requestId: string | undefined
}

export interface AuthEvents {
  reject(rejection: AuthRejection): void
  last(): AuthRejection | undefined
  /** Called when the person tries again, so a stale reason is not shown against a new key. */
  acknowledge(): void
  subscribe(listener: () => void): () => void
}
