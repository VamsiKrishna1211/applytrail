import type { FollowUp } from '@/core/domain/models'
import { urgencyOf } from '@/core/usecases/followupUrgency'
import { toneOfUrgency } from '@/ui/tone'
import { StatusChip } from './StatusChip'
import styles from './FollowupList.module.css'

/**
 * The queue of applications that have gone quiet.
 *
 * Each row opens the application rather than acting on it: chasing a
 * follow-up is a note or a status change, and both belong on the detail
 * drawer where the timeline gives them context.
 */
export function FollowupList({
  followups,
  onOpen,
}: {
  followups: readonly FollowUp[]
  onOpen: (id: string) => void
}) {
  return (
    <div className={styles.list}>
      {followups.map((followUp) => (
        <button
          key={followUp.id}
          type="button"
          className={styles.row}
          data-tone={toneOfUrgency(urgencyOf(followUp))}
          onClick={() => onOpen(followUp.id)}
        >
          <span className={styles.identity}>
            <span className={styles.company}>{followUp.company ?? 'unknown company'}</span>
            <span className={styles.title}>{followUp.title ?? 'untitled posting'}</span>
          </span>

          <StatusChip status={followUp.status} />

          <span className={styles.timing}>
            <span className={styles.overdue}>{followUp.days_overdue}d overdue</span>
            <span className={styles.since}>{followUp.days_since_applied}d since applied</span>
          </span>
        </button>
      ))}
    </div>
  )
}
