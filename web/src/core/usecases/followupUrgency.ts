import type { FollowUp } from '@/core/domain/models'

export type Urgency = 'due' | 'late' | 'critical'

/**
 * How loudly a follow-up should present itself.
 *
 * The thresholds are a display choice, not a rule the server enforces: the API
 * decides what is overdue, this decides what looks alarming. Keeping them here
 * means one edit changes it everywhere.
 */
export function urgencyOf(followUp: Pick<FollowUp, 'days_overdue'>): Urgency {
  if (followUp.days_overdue >= 14) return 'critical'
  if (followUp.days_overdue >= 5) return 'late'
  return 'due'
}

/** Most overdue first; ties broken by how long the application has been open. */
export function byUrgency(a: FollowUp, b: FollowUp): number {
  return b.days_overdue - a.days_overdue || b.days_since_applied - a.days_since_applied
}
