import { allowedTargets, isTerminal, statusLabel, type Status } from '@/core/domain/status'

export interface Move {
  to: Status
  label: string
  /** An interview that stays an interview is another round, and gets its own event. */
  anotherRound: boolean
}

export interface MoveSet {
  moves: Move[]
  terminal: boolean
  hint: string
}

/**
 * The moves to offer from a status.
 *
 * Illegal targets are absent rather than disabled. A greyed-out control invites
 * a click and then explains why it was refused; an absent one never made the
 * promise. The API is the authority either way — this only avoids asking it
 * questions whose answer is already known.
 */
export function describeMoves(from: Status): MoveSet {
  const targets = allowedTargets(from)
  return {
    moves: targets.map((to) => ({
      to,
      label: to === from ? `${statusLabel(to)} (another round)` : statusLabel(to),
      anotherRound: to === from,
    })),
    terminal: isTerminal(from) || targets.length === 0,
    hint: targets.length
      ? 'Anything not listed is rejected with the allowed set in details.'
      : 'Terminal. Every transition from here is rejected.',
  }
}
