import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { Application } from '@/core/domain/models'
import { searchApplications } from '@/core/usecases/searchApplications'
import { useApplications } from '@/ui/hooks/useApplications'
import { cx } from '@/ui/cx'
import { StatusChip } from './StatusChip'
import styles from './GlobalSearch.module.css'

/** Nothing is fetched until this many characters are typed; one letter matches most of the pipeline. */
const MIN_QUERY = 2

/** How many pages the search holds. Beyond this the footer says so rather than lying by omission. */
const PAGE_SIZE = 200

/** Rows in the dropdown. The rest are counted, not listed — a list longer than this is a filter, not an answer. */
const MAX_ROWS = 8

/**
 * Search across the whole pipeline, from anywhere in the app.
 *
 * It searches every status rather than the board's current view, because the
 * point is to reach an application without first knowing where it sits.
 * Choosing a result opens the detail drawer, which is where a status is
 * changed by hand.
 *
 * The query runs against the pages this component has loaded, not the server:
 * the list endpoint's only text parameter is an exact company match, so a
 * substring search has to happen here. One request covers the first 200
 * applications and every keystroke after that is local.
 */
export function GlobalSearch() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const boxRef = useRef<HTMLDivElement>(null)

  const term = query.trim()
  const ready = term.length >= MIN_QUERY

  // Enabled only once there is something to search, so opening the app costs
  // no request; the result is cached, so later keystrokes cost none either.
  const list = useApplications(useMemo(() => ({ limit: PAGE_SIZE }), []), ready)

  const matches = useMemo(
    () => (ready ? searchApplications(list.applications, term) : []),
    [ready, list.applications, term],
  )
  const rows = matches.slice(0, MAX_ROWS)

  // A shrinking result set must not leave the highlight past the end.
  useEffect(() => setActive(0), [term])

  // "/" focuses the box from anywhere, unless the keystroke belongs to a field
  // someone is already typing in.
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.isContentEditable) return
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return
      event.preventDefault()
      inputRef.current?.focus()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])

  function openApplication(application: Application) {
    const next = new URLSearchParams(searchParams)
    next.set('app', application.id)
    setSearchParams(next)
    setOpen(false)
    inputRef.current?.blur()
  }

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'Escape') {
      // Without this, clearing the search box also closed an open drawer:
      // ApplicationDrawer listens for Escape on `document`, so an
      // unstopped keystroke handled here reached it too.
      event.stopPropagation()
      if (query) setQuery('')
      setOpen(false)
      return
    }
    if (!rows.length) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setOpen(true)
      setActive((index) => (index + 1) % rows.length)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((index) => (index - 1 + rows.length) % rows.length)
    } else if (event.key === 'Enter') {
      const row = rows[active]
      if (row) {
        event.preventDefault()
        openApplication(row)
      }
    }
  }

  const showPanel = open && ready

  return (
    <div
      className={styles.search}
      ref={boxRef}
      // Closing on blur would fire before the click that chose a row. Checking
      // where focus went keeps the panel open while it moves inside it.
      onBlur={(event) => {
        if (!boxRef.current?.contains(event.relatedTarget as Node | null)) setOpen(false)
      }}
    >
      <input
        ref={inputRef}
        type="search"
        className={styles.input}
        value={query}
        placeholder="Search company or role"
        aria-label="Search every application"
        autoComplete="off"
        spellCheck={false}
        role="combobox"
        aria-expanded={showPanel}
        aria-controls="global-search-results"
        onChange={(event) => {
          setQuery(event.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
      />
      {query ? (
        <button type="button" className={styles.clear} aria-label="Clear search" onClick={() => setQuery('')}>
          ×
        </button>
      ) : (
        <kbd className={styles.hint}>/</kbd>
      )}

      {showPanel ? (
        <div className={styles.panel} id="global-search-results" role="listbox">
          {list.isPending ? (
            <p className={styles.note}>Loading the pipeline…</p>
          ) : list.isError ? (
            <p className={styles.note}>Could not load applications to search.</p>
          ) : rows.length === 0 ? (
            <p className={styles.note}>No application matches “{term}”.</p>
          ) : (
            rows.map((application, index) => (
              <button
                key={application.id}
                type="button"
                role="option"
                aria-selected={index === active}
                className={cx(styles.row, index === active && styles.rowActive)}
                onMouseEnter={() => setActive(index)}
                onClick={() => openApplication(application)}
              >
                <span className={styles.rowText}>
                  <span className={styles.company}>{application.company ?? 'unknown company'}</span>
                  <span className={styles.role}>{application.title ?? 'untitled posting'}</span>
                </span>
                <StatusChip status={application.status} />
              </button>
            ))
          )}

          {rows.length > 0 || list.hasNextPage ? (
            <div className={styles.footer}>
              <span>
                {matches.length > rows.length
                  ? `${rows.length} of ${matches.length} matches`
                  : `${matches.length} match${matches.length === 1 ? '' : 'es'}`}
                {' in '}
                {list.applications.length} loaded
              </span>
              {list.hasNextPage ? (
                <button
                  type="button"
                  className={styles.more}
                  disabled={list.isFetchingNextPage}
                  onClick={() => void list.fetchNextPage()}
                >
                  {list.isFetchingNextPage ? 'loading…' : 'search older'}
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
