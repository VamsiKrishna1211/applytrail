/**
 * The server's error envelope, as an Error.
 *
 * Every failing endpoint returns the same shape, so every failure reaching the
 * UI is this type. `details` carries the field-level problems the server found
 * in one pass — including, on a rejected transition, the list of statuses that
 * would have been accepted.
 */

export type ApiErrorCode =
  | 'invalid_argument'
  | 'unauthenticated'
  | 'permission_denied'
  | 'not_found'
  | 'method_not_allowed'
  | 'conflict'
  | 'rate_limited'
  | 'internal'
  | 'network'
  // Not a code the server ever sends — deliberately absent from KNOWN_CODES
  // below. It marks a response the client itself could not make sense of
  // (mapping.ts's malformed()): a deterministic parse failure, which retrying
  // the identical request cannot fix. It used to be tagged 'internal', which
  // is retryable, so a guaranteed-to-fail-again parse error was retried
  // three times for nothing.
  | 'malformed_response'

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
    details?: string[]
    request_id?: string
  }
}

const KNOWN_CODES: readonly string[] = [
  'invalid_argument',
  'unauthenticated',
  'permission_denied',
  'not_found',
  'method_not_allowed',
  'conflict',
  'rate_limited',
  'internal',
  'network',
]

export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly status: number
  readonly details: readonly string[]
  readonly requestId: string | undefined
  /** Seconds to wait, from Retry-After. Only ever set on `rate_limited`. */
  readonly retryAfter: number | undefined

  constructor(init: {
    code: ApiErrorCode
    status: number
    message: string
    details?: readonly string[]
    requestId?: string | undefined
    retryAfter?: number | undefined
  }) {
    super(init.message)
    this.name = 'ApiError'
    this.code = init.code
    this.status = init.status
    this.details = init.details ?? []
    this.requestId = init.requestId
    this.retryAfter = init.retryAfter
  }

  /** True when the credential is missing, unknown, revoked or expired. */
  get isAuthFailure(): boolean {
    return this.code === 'unauthenticated'
  }

  /** True when retrying the identical request could plausibly succeed. */
  get isRetryable(): boolean {
    return this.code === 'rate_limited' || this.code === 'internal' || this.code === 'network'
  }

  static fromResponse(status: number, body: unknown, retryAfter?: number | undefined): ApiError {
    const envelope = (body ?? {}) as ErrorEnvelope
    const raw = envelope.error?.code ?? ''
    return new ApiError({
      code: KNOWN_CODES.includes(raw) ? (raw as ApiErrorCode) : 'internal',
      status,
      message: envelope.error?.message ?? `request failed with status ${status}`,
      details: envelope.error?.details ?? [],
      requestId: envelope.error?.request_id,
      retryAfter,
    })
  }

  /** The request never reached the server: offline, DNS, TLS, or an abort we did not ask for. */
  static network(cause: unknown): ApiError {
    return new ApiError({
      code: 'network',
      status: 0,
      message: cause instanceof Error ? cause.message : 'the server could not be reached',
    })
  }
}

export function isApiError(value: unknown): value is ApiError {
  return value instanceof ApiError
}
