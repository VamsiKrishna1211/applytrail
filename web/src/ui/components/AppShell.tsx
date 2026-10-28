import { NavLink, Outlet, useSearchParams } from 'react-router-dom'
import { useClerkState } from '@/app/ClerkSession'
import { useCredential } from '@/app/ServicesProvider'
import { cx } from '@/ui/cx'
import { ApplicationDrawer } from './ApplicationDrawer'
import { GlobalSearch } from './GlobalSearch'
import { ThemeToggle } from './ThemeToggle'
import styles from './AppShell.module.css'

const NAV = [
  { to: '/', label: 'Dashboard', end: true },
  { to: '/pipeline', label: 'Pipeline', end: false },
]

/**
 * The frame every page sits in.
 *
 * Pipeline needs the full window for its columns while the dashboard reads
 * better in a measured column, so the shell takes the width as a prop rather
 * than each page fighting the container it was given.
 *
 * `fill` is the other half of that: a board is a frame whose parts scroll
 * themselves, a dashboard is a document that scrolls as a whole. The two are
 * independent — width is not the same question as height.
 */
export function AppShell({ wide = false, fill = false }: { wide?: boolean; fill?: boolean }) {
  const credential = useCredential()
  const clerk = useClerkState()

  // The open application is a URL parameter, not component state: it survives
  // a reload, it is linkable, and both pages open the same drawer without
  // either owning it.
  const [searchParams, setSearchParams] = useSearchParams()
  const openId = searchParams.get('app') ?? undefined

  return (
    <div className={cx(styles.shell, fill && styles.shellFill)}>
      <header className={styles.topbar}>
        <NavLink to="/" className={cx(styles.brand)}>
          <span className={styles.mark} aria-hidden="true" />
          ApplyTrail
        </NavLink>

        <nav className={styles.nav}>
          {NAV.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end}>
              {item.label}
            </NavLink>
          ))}
        </nav>

        <GlobalSearch />

        <div className={styles.credential}>
          <ThemeToggle />

          {credential.unlocked ? (
            <>
              {/* The last four characters only. A credential that is fully
                  legible on screen is one a screen share leaks. */}
              <span className={styles.token} title="the key this session is using">
                at_live_…{credential.fingerprint}
              </span>
              <button type="button" className={styles.signout} onClick={credential.clear}>
                sign out
              </button>
            </>
          ) : (
            <>
              <span className={styles.token} title="signed in with Clerk">
                {clerk.label ?? 'signed in'}
              </span>
              <button type="button" className={styles.signout} onClick={clerk.signOut}>
                sign out
              </button>
            </>
          )}
        </div>
      </header>

      <main className={cx(styles.main, wide && styles.wide, fill && styles.mainFill)}>
        <Outlet />
      </main>

      {openId ? (
        <ApplicationDrawer
          key={openId}
          id={openId}
          onClose={() => {
            const next = new URLSearchParams(searchParams)
            next.delete('app')
            setSearchParams(next, { replace: true })
          }}
        />
      ) : null}
    </div>
  )
}
