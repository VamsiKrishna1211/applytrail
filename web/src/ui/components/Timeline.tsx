import type { ApplicationEvent, EventType } from '@/core/domain/models'
import { isStatus, statusLabel } from '@/core/domain/status'
import { formatDateTime } from '@/ui/format'
import { toneOf, type Tone } from '@/ui/tone'
import styles from './Timeline.module.css'

const LABELS: Record<EventType, string> = {
  created: 'Application created',
  status_changed: 'Status changed',
  note_added: 'Note added',
  contact_logged: 'Contact logged',
  follow_up_sent: 'Follow-up sent',
  rediscovered: 'Posting resurfaced — same job, no new application',
}

const TONES: Record<EventType, Tone> = {
  created: 'neutral',
  status_changed: 'accent',
  note_added: 'neutral',
  contact_logged: 'neutral',
  follow_up_sent: 'warn',
  rediscovered: 'neutral',
}

/**
 * The append-only history.
 *
 * A status change reads as `from → to`; everything else takes a fixed label.
 * The API does not return event payloads, so the text of a note appears in the
 * notes block rather than under its own entry — the entry is still worth
 * showing, because when a note was written is part of the story.
 */
export function Timeline({ events }: { events: readonly ApplicationEvent[] }) {
  return (
    <div className={styles.timeline}>
      {events.map((event) => (
        <div key={event.id} className={styles.entry} data-tone={toneFor(event)}>
          <div className={styles.rail}>
            <span className={styles.dot} aria-hidden="true" />
            <span className={styles.line} aria-hidden="true" />
          </div>
          <div className={styles.body}>
            <div className={styles.headline}>{headline(event)}</div>
            <div className={styles.meta}>
              <span className={styles.when}>{formatDateTime(event.occurred_at)}</span>
              <span className={styles.actor}>{event.actor || 'system'}</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function headline(event: ApplicationEvent): string {
  if (event.type === 'status_changed' && event.from_status && event.to_status) {
    return `${statusLabel(event.from_status)} → ${statusLabel(event.to_status)}`
  }
  return LABELS[event.type]
}

function toneFor(event: ApplicationEvent): Tone {
  if (event.type === 'status_changed' && event.to_status && isStatus(event.to_status)) {
    return toneOf(event.to_status)
  }
  return TONES[event.type]
}
