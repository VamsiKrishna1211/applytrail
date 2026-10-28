import type { Query, Request, Transport } from '@/core/ports/transport'
import { ApiError } from './ApiError'

/**
 * Resolves the bearer token at call time, so a re-keyed session needs no
 * rewiring. It may be asynchronous: a Clerk session refreshes its token.
 */
export type TokenProvider = () => string | undefined | Promise<string | undefined>

export interface FetchTransportOptions {
  /** API root. Empty string means same-origin, which is what the embedded build serves. */
  baseUrl?: string
  token: TokenProvider
  /** Injected for tests; defaults to the global. */
  fetch?: typeof globalThis.fetch
  /**
   * Called on any 401 with the error itself, so the shell can drop a credential
   * the server no longer accepts *and* say why. A silent drop is not an option:
   * it returns the person to the unlock screen with nothing to act on.
   */
  onAuthFailure?: (error: ApiError) => void
}

/**
 * The one place in the app that knows about HTTP.
 *
 * It owns exactly three concerns: attaching the credential, turning the query
 * object into a query string, and turning a non-2xx response into an ApiError.
 * Anything resource-specific belongs in a repository.
 */
export class FetchTransport implements Transport {
  readonly #baseUrl: string
  readonly #token: TokenProvider
  readonly #fetch: typeof globalThis.fetch
  readonly #onAuthFailure: ((error: ApiError) => void) | undefined

  constructor(options: FetchTransportOptions) {
    this.#baseUrl = (options.baseUrl ?? '').replace(/\/$/, '')
    this.#token = options.token
    this.#fetch = options.fetch ?? globalThis.fetch.bind(globalThis)
    this.#onAuthFailure = options.onAuthFailure
  }

  async send<T>(request: Request): Promise<T> {
    const token = await this.#token()
    const headers = new Headers({ Accept: 'application/json' })
    if (token) {
      // Authorization wins over X-Api-Key server-side; sending one avoids any
      // question of which credential was used.
      headers.set('Authorization', `Bearer ${token}`)
    }
    if (request.body !== undefined) {
      headers.set('Content-Type', 'application/json')
    }

    const url = this.#baseUrl + request.path + encodeQuery(request.query)

    let response: Response
    try {
      response = await this.#fetch(url, {
        method: request.method,
        headers,
        ...(request.body !== undefined ? { body: JSON.stringify(request.body) } : {}),
        ...(request.signal ? { signal: request.signal } : {}),
      })
    } catch (cause) {
      // An abort is the caller's own doing and must stay distinguishable from
      // a dead network, or React Query will treat a cancelled query as failed.
      if (cause instanceof DOMException && cause.name === 'AbortError') throw cause
      throw ApiError.network(cause)
    }

    if (response.status === 204) return undefined as T

    const payload = await readJson(response)

    if (!response.ok) {
      const error = ApiError.fromResponse(response.status, payload, retryAfterOf(response))
      if (error.isAuthFailure) this.#onAuthFailure?.(error)
      throw error
    }
    return payload as T
  }
}

/** A body that is absent or not JSON is not an error by itself — the status decides that. */
async function readJson(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function retryAfterOf(response: Response): number | undefined {
  const header = response.headers.get('Retry-After')
  if (!header) return undefined
  const seconds = Number.parseInt(header, 10)
  return Number.isFinite(seconds) ? seconds : undefined
}

/**
 * Serialises the query.
 *
 * Array values repeat the key (`?status=applied&status=screening`) because
 * that is what the API reads; `undefined` values are dropped so a caller can
 * pass an optional field without composing the object conditionally.
 */
function encodeQuery(query: Query | undefined): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) continue
    for (const item of Array.isArray(value) ? value : [value]) {
      if (item === undefined) continue
      params.append(key, String(item))
    }
  }
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}
