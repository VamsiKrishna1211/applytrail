import type { AuthEvents, AuthRejection } from '@/core/ports/authEvents'

/**
 * Holds the last rejection for the life of the page.
 *
 * Deliberately not persisted: a reason that outlived its reload would be
 * reported against whatever the person types next.
 */
export class InMemoryAuthEvents implements AuthEvents {
  readonly #listeners = new Set<() => void>()
  #last: AuthRejection | undefined

  reject(rejection: AuthRejection): void {
    this.#last = rejection
    this.#emit()
  }

  last(): AuthRejection | undefined {
    return this.#last
  }

  acknowledge(): void {
    if (!this.#last) return
    this.#last = undefined
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
