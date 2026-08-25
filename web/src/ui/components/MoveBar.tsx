import type { Application } from '@/core/domain/models'
import { statusLabel, type Status } from '@/core/domain/status'
import { describeMoves } from '@/core/usecases/describeMoves'
import { toneOf } from '@/ui/tone'
import { ErrorNotice } from './Feedback'
import styles from './MoveBar.module.css'

/**
 * The transitions available from the selected card's status.
 *
 * The chips come from the same graph the server enforces, so a move offered
 * here is a move the API accepts. That is a convenience, not a guarantee: the
 * application may have moved in another tab since this page loaded, and the
 * 409 that follows is reported rather than swallowed.
 */
export function MoveBar({
  application,
  pending,
  error,
  onMove,
  onDeselect,
  onOpen,
}: {
  application: Application | undefined
  pending: boolean
  error: unknown
  onMove: (to: Status) => void
  onDeselect: () => void
  onOpen: () => void
}) {
  if (!application) {
    return (
      <div className={styles.bar}>
        <p className={styles.idle}>Select a card to see the transitions available from its status.</p>
      </div>
    )
  }

  const { moves, terminal, hint } = describeMoves(application.status)

  return (
    <div className={`${styles.bar} ${styles.armed}`}>
      <div className={styles.row}>
        <div className={styles.subject}>
          <div className={styles.name}>
            {application.company ?? 'unknown company'} · {application.title ?? 'untitled posting'}
          </div>
          <div className={styles.current}>
            currently <strong>{statusLabel(application.status)}</strong> ·{' '}
            <button type="button" className={styles.deselect} onClick={onOpen}>
              open timeline
            </button>
          </div>
        </div>

        <div className={styles.moves}>
          <span className="eyebrow">Move to</span>
          {moves.map((move) => (
            <button
              key={move.to}
              type="button"
              className={styles.chip}
              data-tone={toneOf(move.to)}
              disabled={pending}
              onClick={() => onMove(move.to)}
            >
              {move.label}
            </button>
          ))}
          {terminal ? <span className={styles.terminal}>Terminal — the pipeline has ended.</span> : null}
        </div>

        <button type="button" className={styles.deselect} onClick={onDeselect}>
          deselect
        </button>
      </div>

      <p className={styles.hint}>{hint}</p>

      {error ? (
        <div className={styles.failure}>
          <ErrorNotice error={error} what="move this application" />
        </div>
      ) : null}
    </div>
  )
}
