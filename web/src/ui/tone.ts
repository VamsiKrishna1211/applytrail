import type { Status } from '@/core/domain/status'
import type { Urgency } from '@/core/usecases/followupUrgency'

/**
 * What a status looks like.
 *
 * Presentation, not domain: the pipeline graph does not care that an offer is
 * green. Keeping the mapping here means the domain module stays free of colour
 * and this file is the only thing to change when the palette does.
 */
export type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'stop'

const STATUS_TONES: Record<Status, Tone> = {
  discovered: 'neutral',
  screened_out: 'stop',
  queued: 'neutral',
  applied: 'accent',
  screening: 'accent',
  interview: 'accent',
  offer: 'ok',
  accepted: 'ok',
  rejected: 'stop',
  ghosted: 'warn',
  withdrawn: 'stop',
}

export function toneOf(status: Status): Tone {
  return STATUS_TONES[status]
}

const URGENCY_TONES: Record<Urgency, Tone> = {
  due: 'warn',
  late: 'stop',
  critical: 'stop',
}

export function toneOfUrgency(urgency: Urgency): Tone {
  return URGENCY_TONES[urgency]
}
