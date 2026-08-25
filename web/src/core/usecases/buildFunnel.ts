import type { Stats, StatusCount } from '@/core/domain/models'
import { STATUSES, type Status } from '@/core/domain/status'

export interface FunnelRow {
  status: Status
  count: number
  /** 0–1, against the largest bucket. The widest bar is always full. */
  ratio: number
  empty: boolean
}

/**
 * Orders the funnel the way the pipeline runs and scales each bar against the
 * largest bucket.
 *
 * The server reports every status including the empty ones, and they are kept:
 * an empty `offer` row is information, and a funnel that silently drops rows
 * changes shape as data arrives.
 */
export function buildFunnel(stats: Stats): FunnelRow[] {
  const counts = new Map<Status, number>(stats.by_status.map((c: StatusCount) => [c.status, c.count]))
  const max = Math.max(1, ...stats.by_status.map((c) => c.count))

  return STATUSES.map((status) => {
    const count = counts.get(status) ?? 0
    return { status, count, ratio: count / max, empty: count === 0 }
  })
}

/**
 * Scales each variant's response rate against a fixed ceiling rather than
 * against the best performer, so the bars mean the same thing from one render
 * to the next and a uniformly bad set does not display as if one were good.
 */
export const RESPONSE_RATE_CEILING = 0.5

export function responseRateRatio(rate: number): number {
  return Math.min(1, Math.max(0, rate) / RESPONSE_RATE_CEILING)
}
