import { NavLink, Outlet, useSearchParams } from 'react-router-dom'
import { useCredential } from '@/app/ServicesProvider'
import { cx } from '@/ui/cx'
import { ApplicationDrawer } from './ApplicationDrawer'
import { GlobalSearch } from './GlobalSearch'
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
 */
export function AppShell({ wide = false }: { wide?: boolean }) {
  const credential = useCredential()

  // The open application is a URL parameter, not component state: it survives
  // a reload, it is linkable, and both pages open the same drawer without
  // either owning it.
  const [searchParams, setSearchParams] = useSearchParams()
  const openId = searchParams.get('app') ?? undefined

  return (
    <div className={styles.shell}>
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
          {/* The last four characters only. A credential that is fully legible
              on screen is one a screen share leaks. */}
          <span className={styles.token} title="the key this session is using">
            at_live_…{credential.fingerprint}
          </span>
          <button type="button" className={styles.signout} onClick={credential.clear}>
            sign out
          </button>
        </div>
      </header>

      <main className={cx(styles.main, wide && styles.wide)}>
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
