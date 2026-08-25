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

/** The columns of the board, split the way a person thinks about their pipeline. */
export const ACTIVE_BOARD_STATUSES: readonly Status[] = [
  'queued',
  'applied',
  'screening',
  'interview',
  'offer',
]
export const CLOSED_BOARD_STATUSES: readonly Status[] = [
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
  return status.replace('_', ' ')
}
