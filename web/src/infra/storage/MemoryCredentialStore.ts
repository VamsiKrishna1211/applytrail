import type { CredentialStore } from '@/core/ports/credentials'

/** The store with nothing behind it. Used by tests, and by anyone who would rather retype the key. */
export class MemoryCredentialStore implements CredentialStore {
  readonly #listeners = new Set<() => void>()
  #token: string | undefined

  constructor(initial?: string) {
    this.#token = initial
  }

  read(): string | undefined {
    return this.#token
  }

  write(token: string): void {
    this.#token = token
    this.#emit()
  }

  clear(): void {
    this.#token = undefined
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
