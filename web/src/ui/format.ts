import type { Clock } from '@/core/ports/clock'

/**
 * Display formatting.
 *
 * The API speaks RFC 3339 in UTC; people read dates in their own timezone, so
 * the conversion happens once, here, and every component takes the string.
 */

const DATE = new Intl.DateTimeFormat(undefined, { year: 'numeric', month: '2-digit', day: '2-digit' })
const DATE_TIME = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
})

export function formatDate(iso: string | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  return Number.isNaN(date.valueOf()) ? '—' : DATE.format(date)
}

export function formatDateTime(iso: string | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  return Number.isNaN(date.valueOf()) ? '—' : DATE_TIME.format(date)
}

const DAY_MS = 24 * 60 * 60 * 1000

/** Whole days between `iso` and now, floored. Negative values clamp to zero. */
export function daysSince(iso: string | undefined, clock: Clock): number | undefined {
  if (!iso) return undefined
  const then = new Date(iso)
  if (Number.isNaN(then.valueOf())) return undefined
  return Math.max(0, Math.floor((clock.now().valueOf() - then.valueOf()) / DAY_MS))
}

/**
 * How long a card has sat where it is.
 *
 * `updated_at` moves on any write, not only a status change, so this is "days
 * since anything happened" rather than a true time-in-stage. The API does not
 * expose when the current status began; calling it what it is beats printing a
 * number that quietly means something else.
 */
export function dwellLabel(updatedAt: string | undefined, clock: Clock): string {
  const days = daysSince(updatedAt, clock)
  if (days === undefined) return 'no activity recorded'
  if (days === 0) return 'today'
  return `${days}d since last event`
}

/**
 * A posting URL, ready to render as a link.
 *
 * The URL is free text whoever recorded the application supplied, so it is
 * parsed rather than trusted: anything that is not http(s) — a `javascript:`
 * scheme above all — is not offered as a link at all. The label drops the
 * scheme and a leading `www.` because a bar has room for a host and a path,
 * not for `https://`.
 */
export function postingLink(url: string | undefined): { href: string; label: string } | undefined {
  if (!url) return undefined

  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return undefined
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return undefined

  const host = parsed.host.replace(/^www\./, '')
  const path = parsed.pathname === '/' ? '' : parsed.pathname
  return { href: parsed.href, label: `${host}${path}` }
}
