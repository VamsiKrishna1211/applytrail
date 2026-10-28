import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useServices } from '@/app/ServicesProvider'
import { FOLLOWUP_ELIGIBLE_STATUSES, statusLabel, type Status } from '@/core/domain/status'
import { buildFunnel } from '@/core/usecases/buildFunnel'
import { cx } from '@/ui/cx'
import { EmptyState, ErrorNotice, Skeleton } from '@/ui/components/Feedback'
import { FollowupList } from '@/ui/components/FollowupList'
import { Funnel } from '@/ui/components/Funnel'
import { formatDate } from '@/ui/format'
import { Panel, PanelHeader } from '@/ui/components/Panel'
import { StatTile, StatTileGrid } from '@/ui/components/StatTile'
import { VariantTable } from '@/ui/components/VariantTable'
import { useFollowups } from '@/ui/hooks/useFollowups'
import { useStats } from '@/ui/hooks/useStats'
import styles from './DashboardPage.module.css'

export function DashboardPage() {
  const { clock } = useServices()
  const [searchParams, setSearchParams] = useSearchParams()
  const [filter, setFilter] = useState<Status | undefined>(undefined)

  const stats = useStats()
  const followups = useFollowups()

  const funnel = useMemo(() => (stats.data ? buildFunnel(stats.data) : []), [stats.data])

  const visible = useMemo(
    () => (followups.data ?? []).filter((followUp) => !filter || followUp.status === filter),
    [followups.data, filter],
  )

  // Local calendar date, like every other date on this page (formatDate):
  // toISOString().slice(0, 10) printed the UTC date, which reads as
  // tomorrow's date for anyone west of UTC after ~19:00 local.
  const asOf = formatDate(clock.now().toISOString())

  // A frame, not a document: the head and the tiles are fixed rows and every
  // panel below them scrolls inside its own bounds. The funnel is the control
  // that filters the queue beside it, so scrolling one must never take the
  // other off the screen.
  return (
    <div className={styles.page}>
      <div className={styles.head}>
        <h1 className={styles.title}>Pipeline health</h1>
        <span className={styles.asOf}>as of {asOf}</span>
      </div>

      {stats.isError ? (
        <div className={styles.notice}>
          <ErrorNotice error={stats.error} what="load the funnel" />
        </div>
      ) : null}

      <div className={styles.tiles}>
        <StatTileGrid>
          {stats.isPending ? (
            <>
              <Skeleton height={92} />
              <Skeleton height={92} />
              <Skeleton height={92} />
            </>
          ) : stats.data ? (
            <>
              <StatTile
                label="Tracked"
                value={String(stats.data.total_tracked)}
                hint="postings with an application"
              />
              <StatTile
                label="Applied"
                value={String(stats.data.total_applied)}
                hint="reached applied or beyond"
              />
              <StatTile
                label="Response rate"
                value={`${Math.round(stats.data.response_rate * 100)}%`}
                hint={`replies across ${stats.data.total_applied} sent`}
                tone="accent"
              />
            </>
          ) : null}
          {/* Its own load state, independent of `stats`: gating this on
              stats' loading state let it read "Due now: 0" in green while
              follow-ups were themselves still in flight — a false
              nothing-due signal, not an accurate zero. */}
          {followups.isPending ? (
            <Skeleton height={92} />
          ) : followups.isSuccess ? (
            <StatTile
              label="Due now"
              value={String(followups.data.length)}
              hint="quiet past threshold"
              tone={followups.data.length ? 'stop' : 'ok'}
            />
          ) : null}
        </StatTileGrid>
      </div>

      <div className={styles.split}>
        <Panel className={styles.lane}>
          <PanelHeader title="By status" aside="click to filter" />
          <div className={cx(styles.laneBody, 'scrolls')} tabIndex={0} aria-label="Applications by status">
            {stats.isPending ? (
              <Skeleton height={360} />
            ) : stats.data ? (
              <Funnel rows={funnel} selected={filter} onSelect={setFilter} />
            ) : null}
          </div>
        </Panel>

        <Panel className={styles.lane}>
          <PanelHeader
            title="Follow-ups due"
            aside={
              <span className={styles.queueAside}>
                <span className={styles.filterNote}>
                  {filter ? `filtered to ${statusLabel(filter)}` : 'all engaged statuses'}
                </span>
                {filter ? (
                  <button type="button" className={styles.clear} onClick={() => setFilter(undefined)}>
                    clear filter
                  </button>
                ) : null}
              </span>
            }
          />

          <div className={cx(styles.laneBody, 'scrolls')} tabIndex={0} aria-label="Follow-ups due">
            {followups.isError ? (
              <ErrorNotice error={followups.error} what="load the follow-up queue" />
            ) : followups.isPending ? (
              <div className={styles.stack}>
                <Skeleton height={64} />
                <Skeleton height={64} />
                <Skeleton height={64} />
              </div>
            ) : visible.length === 0 && filter && !FOLLOWUP_ELIGIBLE_STATUSES.includes(filter) ? (
              // The funnel is built from every application (stats.by_status);
              // this list is only ever engaged, non-terminal, overdue ones.
              // A row like "rejected" is real in the funnel and can never be
              // real here — say so, rather than implying nothing is overdue.
              <EmptyState>
                {statusLabel(filter)} applications never appear in the follow-up queue.
              </EmptyState>
            ) : visible.length === 0 ? (
              <EmptyState>
                Nothing due here. Terminal applications never appear in this queue.
              </EmptyState>
            ) : (
              <FollowupList
                followups={visible}
                onOpen={(id) => {
                  // Cloned, not replaced: a bare object here would drop any
                  // other query param, same reasoning as GlobalSearch.
                  const next = new URLSearchParams(searchParams)
                  next.set('app', id)
                  setSearchParams(next)
                }}
              />
            )}
          </div>
        </Panel>
      </div>

      <Panel className={cx(styles.lane, styles.variants)}>
        <PanelHeader title="Response rate by resume variant" />
        <div className={cx(styles.laneBody, 'scrolls')} tabIndex={0} aria-label="Response rate by resume variant">
          {stats.isPending ? (
            <Skeleton height={140} />
          ) : stats.data ? (
            stats.data.resume_variants.length === 0 ? (
              <EmptyState>No applications carry a resume variant yet.</EmptyState>
            ) : (
              <VariantTable variants={stats.data.resume_variants} />
            )
          ) : null}
        </div>
      </Panel>
    </div>
  )
}
