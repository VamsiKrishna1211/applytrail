import type { ReactNode } from 'react'
import type { Tone } from '@/ui/tone'
import styles from './StatTile.module.css'

export function StatTileGrid({ children }: { children: ReactNode }) {
  return <div className={styles.grid}>{children}</div>
}

export function StatTile({
  label,
  value,
  hint,
  tone = 'neutral',
}: {
  label: string
  value: string
  hint: string
  tone?: Tone
}) {
  return (
    <div className={styles.tile} data-tone={tone === 'neutral' ? undefined : tone}>
      <div className="eyebrow">{label}</div>
      <div className={styles.value} style={tone === 'neutral' ? { color: 'var(--ink)' } : undefined}>
        {value}
      </div>
      <div className={styles.hint}>{hint}</div>
    </div>
  )
}
