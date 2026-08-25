import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useServices } from '@/app/ServicesProvider'
import { statusLabel, type Status } from '@/core/domain/status'
import { buildFunnel } from '@/core/usecases/buildFunnel'
import { EmptyState, ErrorNotice, Skeleton } from '@/ui/components/Feedback'
import { FollowupList } from '@/ui/components/FollowupList'
import { Funnel } from '@/ui/components/Funnel'
import { Panel, PanelHeader } from '@/ui/components/Panel'
import { StatTile, StatTileGrid } from '@/ui/components/StatTile'
import { VariantTable } from '@/ui/components/VariantTable'
import { useFollowups } from '@/ui/hooks/useFollowups'
import { useStats } from '@/ui/hooks/useStats'
import styles from './DashboardPage.module.css'

export function DashboardPage() {
  const { clock } = useServices()
  const [, setSearchParams] = useSearchParams()
  const [filter, setFilter] = useState<Status | undefined>(undefined)

  const stats = useStats()
  const followups = useFollowups()

  const funnel = useMemo(() => (stats.data ? buildFunnel(stats.data) : []), [stats.data])

  const visible = useMemo(
    () => (followups.data ?? []).filter((followUp) => !filter || followUp.status === filter),
    [followups.data, filter],
  )

  const asOf = clock.now().toISOString().slice(0, 10)

  return (
    <>
      <div className={styles.head}>
        <h1 className={styles.title}>Pipeline health</h1>
        <span className={styles.asOf}>as of {asOf}</span>
      </div>
      <p className={styles.lede}>Every status is reported, including the empty ones.</p>

      {stats.isError ? <ErrorNotice error={stats.error} what="load the funnel" /> : null}

      <StatTileGrid>
        {stats.isPending || !stats.data ? (
          <>
            <Skeleton height={104} />
            <Skeleton height={104} />
            <Skeleton height={104} />
            <Skeleton height={104} />
          </>
        ) : (
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
            <StatTile
              label="Due now"
              value={String(followups.data?.length ?? 0)}
              hint="quiet past threshold"
              tone={followups.data?.length ? 'stop' : 'ok'}
            />
          </>
        )}
      </StatTileGrid>

      <div className={`${styles.section} ${styles.split}`}>
        <Panel>
          <PanelHeader title="By status" aside="click to filter" />
          {stats.data ? (
            <Funnel rows={funnel} selected={filter} onSelect={setFilter} />
          ) : (
            <Skeleton height={330} />
          )}
        </Panel>

        <div>
          <div className={styles.queueHead}>
            <h2 className="eyebrow">Follow-ups due</h2>
            <span className={styles.filterNote}>
              {filter ? `filtered to ${statusLabel(filter)}` : 'all engaged statuses'}
            </span>
            {filter ? (
              <button type="button" className={styles.clear} onClick={() => setFilter(undefined)}>
                clear filter
              </button>
            ) : null}
          </div>

          {followups.isError ? (
            <ErrorNotice error={followups.error} what="load the follow-up queue" />
          ) : followups.isPending ? (
            <div className={styles.stack}>
              <Skeleton height={64} />
              <Skeleton height={64} />
              <Skeleton height={64} />
            </div>
          ) : visible.length === 0 ? (
            <EmptyState>
              Nothing due here. Terminal applications never appear in this queue.
            </EmptyState>
          ) : (
            <FollowupList
              followups={visible}
              onOpen={(id) => setSearchParams({ app: id })}
            />
          )}
        </div>
      </div>

      <div className={styles.section}>
        <Panel>
          <PanelHeader title="Response rate by resume variant" />
          {stats.isPending || !stats.data ? (
            <Skeleton height={140} />
          ) : stats.data.resume_variants.length === 0 ? (
            <EmptyState>No applications carry a resume variant yet.</EmptyState>
          ) : (
            <VariantTable variants={stats.data.resume_variants} />
          )}
        </Panel>
      </div>
    </>
  )
}
