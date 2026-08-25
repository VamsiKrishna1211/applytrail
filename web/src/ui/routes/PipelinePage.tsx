import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useServices } from '@/app/ServicesProvider'
import {
  ACTIVE_BOARD_STATUSES,
  CLOSED_BOARD_STATUSES,
  type Status,
} from '@/core/domain/status'
import { buildBoard } from '@/core/usecases/buildBoard'
import { Board } from '@/ui/components/Board'
import { ErrorNotice, Skeleton } from '@/ui/components/Feedback'
import { MoveBar } from '@/ui/components/MoveBar'
import { dwellLabel } from '@/ui/format'
import { useApplications } from '@/ui/hooks/useApplications'
import { useUpdateStatus } from '@/ui/hooks/useApplicationMutations'
import styles from './PipelinePage.module.css'

type View = 'active' | 'closed'

const VIEWS: Record<View, { label: string; statuses: readonly Status[] }> = {
  active: { label: 'Active', statuses: ACTIVE_BOARD_STATUSES },
  closed: { label: 'Closed', statuses: CLOSED_BOARD_STATUSES },
}

/** One page of cards. Large enough that most boards arrive whole, bounded so none arrives forever. */
const PAGE_SIZE = 100

export function PipelinePage() {
  const { clock } = useServices()
  const [, setSearchParams] = useSearchParams()
  const [view, setView] = useState<View>('active')
  const [selectedId, setSelectedId] = useState<string | undefined>(undefined)

  const statuses = VIEWS[view].statuses
  const query = useMemo(() => ({ status: statuses, limit: PAGE_SIZE }), [statuses])
  const list = useApplications(query)
  const updateStatus = useUpdateStatus()

  const columns = useMemo(
    () => buildBoard(list.applications, statuses),
    [list.applications, statuses],
  )
  const selected = list.applications.find((application) => application.id === selectedId)

  function switchView(next: View) {
    setView(next)
    setSelectedId(undefined)
    updateStatus.reset()
  }

  function move(to: Status) {
    if (!selectedId) return
    updateStatus.mutate(
      { id: selectedId, change: { status: to } },
      {
        // A card that leaves this view has nothing left to select. One that
        // stays keeps its selection, so several moves in a row need no
        // re-clicking.
        onSuccess: (application) => {
          if (!statuses.includes(application.status)) setSelectedId(undefined)
        },
      },
    )
  }

  return (
    <>
      <div className={styles.head}>
        <div>
          <h1 className={styles.title}>Pipeline</h1>
          <p className={styles.lede}>
            Selecting a card offers only the moves the API will accept from its status.
          </p>
        </div>

        <div className={styles.segmented} role="group" aria-label="Which part of the pipeline">
          {(Object.keys(VIEWS) as View[]).map((key) => (
            <button
              key={key}
              type="button"
              className={styles.segment}
              aria-pressed={view === key}
              onClick={() => switchView(key)}
            >
              {VIEWS[key].label}
            </button>
          ))}
        </div>
      </div>

      {list.isError ? <ErrorNotice error={list.error} what="load the board" /> : null}

      {list.isPending ? (
        <div className={styles.loading}>
          <Skeleton height={220} />
          <Skeleton height={220} />
          <Skeleton height={220} />
          <Skeleton height={220} />
          <Skeleton height={220} />
        </div>
      ) : (
        <Board
          columns={columns}
          selectedId={selectedId}
          onSelect={setSelectedId}
          daysIn={(application) => dwellLabel(application.updated_at, clock)}
        />
      )}

      {list.hasNextPage ? (
        <div className={styles.more}>
          <button
            type="button"
            className={styles.moreButton}
            disabled={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            {list.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
          <span className={styles.moreNote}>
            Showing {list.applications.length}; the board is not complete until this is empty.
          </span>
        </div>
      ) : null}

      <div className={styles.bar}>
        <MoveBar
          application={selected}
          pending={updateStatus.isPending}
          error={updateStatus.error}
          onMove={move}
          onDeselect={() => {
            setSelectedId(undefined)
            updateStatus.reset()
          }}
          onOpen={() => selectedId && setSearchParams({ app: selectedId })}
        />
      </div>
    </>
  )
}
