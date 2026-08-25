/**
 * The transport seam.
 *
 * Repositories depend on this, never on `fetch`. That is what lets a
 * repository be tested against a stub, and what would let the whole app move
 * to a different transport without a repository changing.
 */

export type QueryValue = string | number | boolean | undefined
export type Query = Record<string, QueryValue | readonly QueryValue[]>

export interface Request {
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  /** Path below the API root, e.g. `/v1/applications`. */
  path: string
  query?: Query
  body?: unknown
  signal?: AbortSignal
}

export interface Transport {
  /**
   * Performs the request and decodes the JSON body.
   *
   * Rejects with an ApiError carrying the server's error envelope for any
   * non-2xx response, so a caller never has to inspect a status code.
   */
  send<T>(request: Request): Promise<T>
}
