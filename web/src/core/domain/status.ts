/**
 * The pipeline status graph, mirroring internal/domain/application.go.
 *
 * This is the one piece of Go logic the browser has to know, because a board
 * that offers a move the API will reject is worse than no board at all. The
 * table below is parsed by TestWebUITransitionTableMatchesDomain, which fails
 * the Go build if the two ever disagree — so edit the Go file first, then this
 * one.
 */

export const STATUSES = [
  'discovered',
  'screened_out',
  'queued',
  'applied',
  'screening',
  'interview',
  'offer',
  'accepted',
  'rejected',
  'ghosted',
  'withdrawn',
] as const

export type Status = (typeof STATUSES)[number]

/** ALLOWED_TRANSITIONS: parsed by the Go drift test — keep one status per line. */
export const ALLOWED_TRANSITIONS: Readonly<Record<Status, readonly Status[]>> = {
  discovered: ['screened_out', 'queued', 'applied', 'withdrawn'],
  screened_out: ['queued', 'applied'],
  queued: ['applied', 'screened_out', 'withdrawn'],
  applied: ['screening', 'interview', 'offer', 'rejected', 'ghosted', 'withdrawn'],
  screening: ['interview', 'offer', 'rejected', 'ghosted', 'withdrawn'],
  interview: ['interview', 'offer', 'rejected', 'ghosted', 'withdrawn'],
  offer: ['accepted', 'rejected', 'withdrawn'],
  ghosted: ['screening', 'interview', 'offer', 'rejected', 'withdrawn'],
  accepted: [],
  rejected: [],
  withdrawn: [],
}

/** Statuses the pipeline has ended for. They never resurface, and never appear in the follow-up queue. */
export const TERMINAL_STATUSES: readonly Status[] = ['accepted', 'rejected', 'withdrawn']

/** Statuses where the tenant has actually applied. Follow-up SLAs act on these. */
export const ENGAGED_STATUSES: readonly Status[] = ['applied', 'screening', 'interview', 'offer']

/**
 * Statuses ListFollowupsDue's WHERE clause admits (internal/db/queries/applications.sql).
 * Deliberately not the same list as ENGAGED_STATUSES: 'ghosted' can carry a
 * follow-up date (it is re-enterable — an employer can resurface after
 * months of silence) even though it is not "engaged" in the IsEngaged sense.
 * Used to tell a genuinely-empty follow-up queue apart from one that is
 * empty because the selected status structurally can never appear there.
 */
export const FOLLOWUP_ELIGIBLE_STATUSES: readonly Status[] = [
  'applied',
  'screening',
  'interview',
  'offer',
  'ghosted',
]

/**
 * The columns of the board, split the way a person thinks about their pipeline.
 *
 * Between them the two views name every status in `STATUSES`. A status in
 * neither is a status whose cards exist and cannot be reached: `discovered` in
 * particular is where `record_discoveries` puts everything an agent finds, so
 * leaving it off the board hid the whole intake.
 *
 * `screened_out` sits with Closed rather than Active because it is a decision
 * not to pursue. It is not terminal — `screened_out` can still go to `queued`
 * or `applied` — so it is reachable from where it lives, not stranded there.
 */
export const ACTIVE_BOARD_STATUSES: readonly Status[] = [
  'discovered',
  'queued',
  'applied',
  'screening',
  'interview',
  'offer',
]
export const CLOSED_BOARD_STATUSES: readonly Status[] = [
  'screened_out',
  'accepted',
  'rejected',
  'ghosted',
  'withdrawn',
]

export function isStatus(value: string): value is Status {
  return (STATUSES as readonly string[]).includes(value)
}

export function isTerminal(status: Status): boolean {
  return TERMINAL_STATUSES.includes(status)
}

export function isEngaged(status: Status): boolean {
  return ENGAGED_STATUSES.includes(status)
}

/** The moves the API will accept from `status`. Anything else is a 400. */
export function allowedTargets(status: Status): readonly Status[] {
  return ALLOWED_TRANSITIONS[status]
}

export function canTransition(from: Status, to: Status): boolean {
  return ALLOWED_TRANSITIONS[from].includes(to)
}

/** Human label. The wire format is snake_case; nothing but this function should undo that. */
export function statusLabel(status: Status): string {
  // replaceAll, not replace: every status today has at most one underscore,
  // but replace() only fixing the first would silently mangle the day a
  // two-underscore status (e.g. a hypothetical "follow_up_sent") is added.
  return status.replaceAll('_', ' ')
}
