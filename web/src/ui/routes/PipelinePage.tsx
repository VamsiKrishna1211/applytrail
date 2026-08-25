import { useMemo, useState, type CSSProperties } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useServices } from '@/app/ServicesProvider'
import {
  ACTIVE_BOARD_STATUSES,
  CLOSED_BOARD_STATUSES,
  type Status,
} from '@/core/domain/status'
import { buildBoard } from '@/core/usecases/buildBoard'
import { searchApplications } from '@/core/usecases/searchApplications'
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
  const [filter, setFilter] = useState('')
  const [selectedId, setSelectedId] = useState<string | undefined>(undefined)

  const statuses = VIEWS[view].statuses
  const query = useMemo(() => ({ status: statuses, limit: PAGE_SIZE }), [statuses])
  const list = useApplications(query)
  const updateStatus = useUpdateStatus()

  // The same matcher the top-bar search uses, over the cards already on the
  // board: this narrows what is in front of you, where that one finds an
  // application in any status anywhere in the pipeline.
  const shown = useMemo(
    () => searchApplications(list.applications, filter),
    [list.applications, filter],
  )

  const columns = useMemo(() => buildBoard(shown, statuses), [shown, statuses])

  // Selection is read out of the filtered set, so a card the filter hides
  // takes the move bar with it and gets it back when the filter is cleared.
  const selected = shown.find((application) => application.id === selectedId)

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

  // A frame, not a document: the head and the move bar are fixed rows and the
  // board between them takes whatever height is left. Nothing here scrolls —
  // the columns do, one at a time.
  return (
    <div className={styles.page}>
      <div className={styles.head}>
        <h1 className={styles.title}>Pipeline</h1>

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

        <div className={styles.filter}>
          <input
            type="search"
            className={styles.filterInput}
            value={filter}
            placeholder="Filter this board"
            aria-label="Filter the cards on this board"
            autoComplete="off"
            spellCheck={false}
            onChange={(event) => setFilter(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') setFilter('')
            }}
          />
          {filter ? (
            <button
              type="button"
              className={styles.filterClear}
              aria-label="Clear the filter"
              onClick={() => setFilter('')}
            >
              ×
            </button>
          ) : null}
        </div>

        {/* What the filter is hiding, said plainly — a board with columns of
            zero otherwise looks like a board with nothing on it. */}
        {filter ? (
          <span className={styles.filterCount}>
            {shown.length} of {list.applications.length}
          </span>
        ) : null}

        {/* Paging belongs beside the count, not under a board that now ends
            with the window. */}
        {list.hasNextPage ? (
          <div className={styles.more}>
            <span className={styles.moreNote}>
              Showing {list.applications.length}; the board is not complete until this is empty.
            </span>
            <button
              type="button"
              className={styles.moreButton}
              disabled={list.isFetchingNextPage}
              onClick={() => void list.fetchNextPage()}
            >
              {list.isFetchingNextPage ? 'Loading…' : 'Load more'}
            </button>
          </div>
        ) : null}
      </div>

      {list.isError ? <ErrorNotice error={list.error} what="load the board" /> : null}

      {list.isPending ? (
        <div className={styles.loading} style={{ '--columns': statuses.length } as CSSProperties}>
          {statuses.map((status) => (
            <Skeleton key={status} height={220} />
          ))}
        </div>
      ) : (
        <Board
          columns={columns}
          selectedId={selectedId}
          onSelect={setSelectedId}
          daysIn={(application) => dwellLabel(application.updated_at, clock)}
        />
      )}

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
    </div>
  )
}
