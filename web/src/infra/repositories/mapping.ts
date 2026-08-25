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
  FollowUp,
  Stats,
} from '@/core/domain/models'
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

function malformed(message: string): ApiError {
  return new ApiError({ code: 'internal', status: 0, message })
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

export function toEvent(wire: unknown): ApplicationEvent {
  const value = wire as ApplicationEvent
  return {
    ...value,
    id: requireId(value, 'event'),
    type: value.type ?? 'created',
    occurred_at: value.occurred_at ?? '',
  }
}

export function toDetail(wire: unknown): ApplicationDetail {
  const value = wire as { application?: unknown; events?: unknown[]; contacts?: Contact[] }
  return {
    application: toApplication(value.application),
    events: (value.events ?? []).map(toEvent),
    contacts: value.contacts ?? [],
  }
}

export function toStats(wire: unknown): Stats {
  const value = wire as Stats
  return {
    by_status: (value.by_status ?? [])
      .filter((entry) => isStatus(String(entry.status)))
      .map((entry) => ({ status: entry.status, count: entry.count ?? 0 })),
    resume_variants: value.resume_variants ?? [],
    total_tracked: value.total_tracked ?? 0,
    total_applied: value.total_applied ?? 0,
    response_rate: value.response_rate ?? 0,
  }
}
