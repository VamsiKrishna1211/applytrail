import type { CredentialStore } from '@/core/ports/credentials'

const KEY = 'applytrail.credential'

/**
 * Keeps the API key in sessionStorage.
 *
 * sessionStorage rather than localStorage is the whole point: the credential
 * dies with the tab, so a shared or forgotten machine does not keep a live
 * key. It is still readable by any script running on this origin, which is why
 * the UI should be given a read+write key and never an admin one.
 *
 * Storage can throw outright — Safari in private mode, a browser configured to
 * block site data — so every access is guarded and the store degrades to
 * in-memory rather than taking the app down.
 */
export class SessionCredentialStore implements CredentialStore {
  readonly #listeners = new Set<() => void>()
  readonly #storage: Storage | undefined
  #fallback: string | undefined

  constructor(storage: Storage | undefined = safeSessionStorage()) {
    this.#storage = storage
  }

  read(): string | undefined {
    if (!this.#storage) return this.#fallback
    try {
      return this.#storage.getItem(KEY) ?? undefined
    } catch {
      return this.#fallback
    }
  }

  write(token: string): void {
    this.#fallback = token
    try {
      this.#storage?.setItem(KEY, token)
    } catch {
      // Held in memory for this page instead; nothing else to do.
    }
    this.#emit()
  }

  clear(): void {
    this.#fallback = undefined
    try {
      this.#storage?.removeItem(KEY)
    } catch {
      // Same as write: the in-memory copy is already gone.
    }
    this.#emit()
  }

  subscribe(listener: () => void): () => void {
    this.#listeners.add(listener)
    return () => {
      this.#listeners.delete(listener)
    }
  }

  #emit(): void {
    for (const listener of this.#listeners) listener()
  }
}

function safeSessionStorage(): Storage | undefined {
  try {
    return globalThis.sessionStorage
  } catch {
    return undefined
  }
}
