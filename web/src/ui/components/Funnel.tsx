import { statusLabel, type Status } from '@/core/domain/status'
import type { FunnelRow } from '@/core/usecases/buildFunnel'
import { toneOf } from '@/ui/tone'
import styles from './Funnel.module.css'

/**
 * The funnel, every status of it.
 *
 * Each row is a toggle that filters the follow-up queue beside it. Empty rows
 * stay clickable: filtering to `offer` and seeing an empty queue is an answer,
 * and a control that appears and disappears with the data is worse than one
 * that is simply quiet.
 */
export function Funnel({
  rows,
  selected,
  onSelect,
}: {
  rows: readonly FunnelRow[]
  selected: Status | undefined
  onSelect: (status: Status | undefined) => void
}) {
  return (
    <div className={styles.rows}>
      {rows.map((row) => {
        const active = selected === row.status
        return (
          <button
            key={row.status}
            type="button"
            className={row.empty ? `${styles.row} ${styles.empty}` : styles.row}
            data-tone={toneOf(row.status)}
            aria-pressed={active}
            onClick={() => onSelect(active ? undefined : row.status)}
          >
            <span className={styles.label}>{statusLabel(row.status)}</span>
            <span className={styles.track}>
              <span className={styles.bar} style={{ width: `${Math.round(row.ratio * 100)}%` }} />
            </span>
            <span className={styles.count}>{row.count}</span>
          </button>
        )
      })}
    </div>
  )
}
