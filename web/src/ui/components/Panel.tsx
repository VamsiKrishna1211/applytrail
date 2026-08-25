import type { ReactNode } from 'react'
import { cx } from '@/ui/cx'
import styles from './Panel.module.css'

/** A bordered surface. The one place the card border, radius and shadow are decided. */
export function Panel({
  children,
  padded = true,
  className,
}: {
  children: ReactNode
  padded?: boolean
  className?: string | undefined
}) {
  return (
    <section
      className={cx(styles.panel, padded && styles.padded, className)}
    >
      {children}
    </section>
  )
}

export function PanelHeader({ title, aside }: { title: string; aside?: ReactNode }) {
  return (
    <header className={styles.header}>
      <h2 className="eyebrow">{title}</h2>
      {aside ? <div className={styles.aside}>{aside}</div> : null}
    </header>
  )
}
