import type { IdentityProvider } from '@/core/ports/identity'

/** The slice of Clerk's browser object this adapter calls. */
export interface ClerkLike {
  readonly session?: { getToken(): Promise<string | null> } | null | undefined
  signOut(): Promise<unknown>
}

/**
 * Clerk as an IdentityProvider.
 *
 * The transport is built before React renders, but Clerk is created by its
 * React provider and only usable once it has loaded, so the provider hands it
 * over through attach(). Until then token() waits instead of answering
 * "signed out": the shell sends nothing before it knows who is signed in, but
 * a request that slipped out early must not go out bare and be refused.
 */
export class ClerkIdentity implements IdentityProvider {
  #clerk: ClerkLike | undefined
  #attached: (clerk: ClerkLike) => void = () => {}
  readonly #ready = new Promise<ClerkLike>((resolve) => {
    this.#attached = resolve
  })

  attach(clerk: ClerkLike): void {
    this.#clerk = clerk
    this.#attached(clerk)
  }

  async token(): Promise<string | undefined> {
    const clerk = this.#clerk ?? (await this.#ready)
    // getToken returns a cached token until shortly before it expires, then
    // refreshes it, so asking on every request costs nothing.
    return (await clerk.session?.getToken()) ?? undefined
  }

  async signOut(): Promise<void> {
    const clerk = this.#clerk ?? (await this.#ready)
    await clerk.signOut()
  }
}
