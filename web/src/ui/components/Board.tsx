import type { Application } from '@/core/domain/models'
import { statusLabel } from '@/core/domain/status'
import type { BoardColumn } from '@/core/usecases/buildBoard'
import { toneOf } from '@/ui/tone'
import styles from './Board.module.css'

export function Board({
  columns,
  selectedId,
  onSelect,
  daysIn,
}: {
  columns: readonly BoardColumn[]
  selectedId: string | undefined
  onSelect: (id: string | undefined) => void
  daysIn: (application: Application) => string
}) {
  return (
    <div
      className={styles.board}
      style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(0, 1fr))` }}
    >
      {columns.map((column) => (
        <div key={column.status} className={styles.column} data-tone={toneOf(column.status)}>
          <div className={styles.columnHead}>
            <span className={styles.dot} aria-hidden="true" />
            <span className={styles.columnName}>{statusLabel(column.status)}</span>
            <span className={styles.columnCount}>{column.applications.length}</span>
          </div>

          {column.applications.map((application) => (
            <button
              key={application.id}
              type="button"
              className={styles.card}
              aria-pressed={application.id === selectedId}
              onClick={() => onSelect(application.id === selectedId ? undefined : application.id)}
            >
              <span className={styles.cardCompany}>{application.company ?? 'unknown company'}</span>
              <span className={styles.cardTitle}>{application.title ?? 'untitled posting'}</span>
              <span className={styles.cardMeta}>
                <span className={styles.age}>{daysIn(application)}</span>
                {application.resume_variant ? (
                  <span className={styles.variant}>{application.resume_variant}</span>
                ) : null}
              </span>
            </button>
          ))}

          {column.applications.length === 0 ? <p className={styles.columnEmpty}>empty</p> : null}
        </div>
      ))}
    </div>
  )
}
