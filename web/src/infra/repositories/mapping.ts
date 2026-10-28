/**
 * Wire → domain narrowing.
 *
 * The generated types make every field optional, because the OpenAPI document
 * describes a response that omits empty values. The UI wants the opposite: the
 * few fields the server always sends should be non-optional, and `status`
 * should be the Status union rather than a bare string.
 *
 * An unrecognised status is a real signal — the server has grown a status this
 * build does not know — so it fails loudly here rather than rendering a card
 * into a column that does not exist.
 */

import type {
  Application,
  ApplicationDetail,
  ApplicationEvent,
  Contact,
  EventType,
  FollowUp,
  ResumeVariantStats,
  Stats,
} from '@/core/domain/models'
import { EVENT_TYPES } from '@/core/domain/models'
import { isStatus, type Status } from '@/core/domain/status'
import { ApiError } from '@/infra/http/ApiError'

interface WireIdentified {
  id?: string
}

function requireId(value: WireIdentified, what: string): string {
  if (!value.id) {
    throw malformed(`${what} arrived without an id`)
  }
  return value.id
}

function requireStatus(raw: string | undefined, what: string): Status {
  if (!raw || !isStatus(raw)) {
    throw malformed(`${what} has unrecognised status "${raw ?? ''}"`)
  }
  return raw
}

function isEventType(value: string): value is EventType {
  return (EVENT_TYPES as readonly string[]).includes(value)
}

// Not code 'internal': that means "the server itself failed" and is
// retryable. This means "the client could not make sense of what the server
// sent" — a parse failure retrying the identical request cannot fix, so it
// must not carry a retryable code the way it used to.
function malformed(message: string): ApiError {
  return new ApiError({ code: 'malformed_response', status: 0, message })
}

export function toApplication(wire: unknown): Application {
  const value = wire as Application
  return {
    ...value,
    id: requireId(value, 'application'),
    status: requireStatus(value.status, 'application'),
  }
}

export function toFollowUp(wire: unknown): FollowUp {
  const value = wire as FollowUp
  return {
    ...toApplication(value),
    days_since_applied: value.days_since_applied ?? 0,
    days_overdue: value.days_overdue ?? 0,
  }
}

/**
 * Returns undefined, not a fabricated 'created', for an event whose type this
 * build does not recognise. One stale historical event of an unknown type
 * used to silently masquerade as the entry that opens every timeline; toDetail
 * below drops it instead, with a console warning, rather than lying about
 * what happened.
 */
export function toEvent(wire: unknown): ApplicationEvent | undefined {
  const value = wire as ApplicationEvent
  if (!value.type || !isEventType(value.type)) {
    console.warn('dropping application event with unrecognised type', value.id, value.type)
    return undefined
  }
  return {
    ...value,
    id: requireId(value, 'event'),
    type: value.type,
    occurred_at: value.occurred_at ?? '',
  }
}

export function toDetail(wire: unknown): ApplicationDetail {
  const value = wire as { application?: unknown; events?: unknown[]; contacts?: Contact[] }
  return {
    application: toApplication(value.application),
    events: (value.events ?? []).map(toEvent).filter((event) => event !== undefined),
    contacts: value.contacts ?? [],
  }
}

/** A variant row with every field present and numeric, or undefined. Numbers
 * arrive as `number | undefined` on the wire; leaving a gap unfilled instead
 * of defaulting it to 0 used to reach VariantTable as `undefined * 100`,
 * rendered as `NaN%` and `width: 'NaN%'`. */
function toResumeVariant(wire: unknown): ResumeVariantStats | undefined {
  const value = wire as Partial<ResumeVariantStats>
  if (!value.variant) return undefined
  return {
    variant: value.variant,
    sent: value.sent ?? 0,
    responded: value.responded ?? 0,
    rejected: value.rejected ?? 0,
    ghosted: value.ghosted ?? 0,
    response_rate: value.response_rate ?? 0,
  }
}

export function toStats(wire: unknown): Stats {
  const value = wire as Stats
  return {
    by_status: (value.by_status ?? [])
      .filter((entry) => isStatus(String(entry.status)))
      .map((entry) => ({ status: entry.status, count: entry.count ?? 0 })),
    resume_variants: (value.resume_variants ?? [])
      .map(toResumeVariant)
      .filter((variant) => variant !== undefined),
    total_tracked: value.total_tracked ?? 0,
    total_applied: value.total_applied ?? 0,
    response_rate: value.response_rate ?? 0,
  }
}
