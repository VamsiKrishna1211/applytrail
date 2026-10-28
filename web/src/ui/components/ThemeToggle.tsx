import { useEffect, useState } from 'react'
import styles from './ThemeToggle.module.css'

const KEY = 'applytrail.theme'

type Theme = 'system' | 'light' | 'dark'

/** system → light → dark → system. Three states, because "follow the system"
 *  is the default and dropping it would take away behaviour people have. */
const NEXT: Record<Theme, Theme> = { system: 'light', light: 'dark', dark: 'system' }

const LABEL: Record<Theme, string> = {
  system: 'following your system',
  light: 'light',
  dark: 'dark',
}

function read(): Theme {
  try {
    const stored = localStorage.getItem(KEY)
    return stored === 'light' || stored === 'dark' ? stored : 'system'
  } catch {
    // Storage can throw outright — private mode, blocked site data. The system
    // scheme is a good answer; the choice just will not survive the reload.
    return 'system'
  }
}

/**
 * The scheme control.
 *
 * It writes one attribute and one stored string; every colour in the app comes
 * from tokens keyed off that attribute, so nothing else in the tree knows a
 * theme changed. `public/theme.js` reads the same key before the first paint,
 * which is what keeps a chosen theme from flashing the other one on load.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(read)

  useEffect(() => {
    const root = document.documentElement
    if (theme === 'system') root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', theme)

    try {
      if (theme === 'system') localStorage.removeItem(KEY)
      else localStorage.setItem(KEY, theme)
    } catch {
      // Applied for this page either way; only persistence is lost.
    }
  }, [theme])

  // localStorage is shared across tabs, but the `storage` event that reports
  // a change fires only in *other* tabs, not the one that wrote it — so
  // without this, toggling to dark in tab B left tab A on light until it was
  // reloaded, even though both tabs agree on what is actually stored.
  useEffect(() => {
    function onStorage(event: StorageEvent) {
      if (event.key === KEY || event.key === null) setTheme(read())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  return (
    <button
      type="button"
      className={styles.toggle}
      onClick={() => setTheme(NEXT[theme])}
      title={`Theme: ${LABEL[theme]} — switch to ${LABEL[NEXT[theme]]}`}
      aria-label={`Theme: ${LABEL[theme]}. Switch to ${LABEL[NEXT[theme]]}.`}
    >
      {theme === 'system' ? <SystemIcon /> : theme === 'light' ? <SunIcon /> : <MoonIcon />}
    </button>
  )
}

/* Drawn rather than borrowed: a 16px stroke set on the same grid, so they take
   the button's colour and stay sharp at any zoom. */

function icon(children: React.ReactNode) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {children}
    </svg>
  )
}

function SunIcon() {
  return icon(
    <>
      <circle cx="8" cy="8" r="3.1" />
      <path d="M8 1.4v1.5M8 13.1v1.5M14.6 8h-1.5M2.9 8H1.4M12.67 3.33l-1.06 1.06M4.39 11.61l-1.06 1.06M12.67 12.67l-1.06-1.06M4.39 4.39L3.33 3.33" />
    </>,
  )
}

function MoonIcon() {
  return icon(<path d="M13.5 9.6A5.9 5.9 0 0 1 6.4 2.5a5.9 5.9 0 1 0 7.1 7.1z" />)
}

/** Half lit, half not: the scheme is whatever the machine says today. */
function SystemIcon() {
  return icon(
    <>
      <circle cx="8" cy="8" r="6" />
      <path d="M8 2v12" />
      <path d="M8 3.6a4.4 4.4 0 0 1 0 8.8z" fill="currentColor" stroke="none" />
    </>,
  )
}
