/**
 * The shapes the UI works in.
 *
 * Every one is derived from the generated OpenAPI schema rather than retyped,
 * so a field renamed in api/openapi.yaml breaks the build here instead of
 * rendering blank at runtime. The two differences from the wire types are
 * deliberate: statuses narrow to the Status union, and fields the server
 * always sends stop being optional.
 */

import type { components } from '@/api/schema'
import type { Status } from './status'

type Schemas = components['schemas']

type WireApplication = Schemas['Application']
type WireEvent = Schemas['Event']
type WireContact = Schemas['Contact']

export interface Application extends Omit<WireApplication, 'id' | 'status'> {
  id: string
  status: Status
}

export interface ApplicationEvent extends Omit<WireEvent, 'id' | 'type' | 'occurred_at'> {
  id: string
  type: EventType
  occurred_at: string
}

export type EventType = NonNullable<WireEvent['type']>

/**
 * Mirrors api/openapi.yaml's Event.type enum. There is no generated runtime
 * list to check against (openapi-typescript only emits the type-level
 * union), so this is hand-kept — the one place mapping.ts's isEventType()
 * needs to agree with the schema.
 */
export const EVENT_TYPES: readonly EventType[] = [
  'created',
  'status_changed',
  'note_added',
  'contact_logged',
  'follow_up_sent',
  'rediscovered',
]

export type Contact = WireContact

export interface FollowUp extends Application {
  days_since_applied: number
  days_overdue: number
}

export interface ApplicationDetail {
  application: Application
  events: ApplicationEvent[]
  contacts: Contact[]
}

export interface StatusCount {
  status: Status
  count: number
}

export interface ResumeVariantStats {
  variant: string
  sent: number
  responded: number
  rejected: number
  ghosted: number
  response_rate: number
}

export interface Stats {
  by_status: StatusCount[]
  resume_variants: ResumeVariantStats[]
  total_tracked: number
  total_applied: number
  response_rate: number
}

/** A page of results plus the cursor that fetches the next one. Absent cursor means the end. */
export interface Page<T> {
  items: T[]
  nextCursor: string | undefined
}

export interface ListApplicationsQuery {
  status?: readonly Status[]
  company?: string
  since?: string
  limit?: number
  cursor?: string
}

export interface StatusChange {
  status: Status
  occurred_at?: string
  note?: string
  next_action_at?: string
  clear_next_action?: boolean
}
