import { statusLabel, type Status } from '@/core/domain/status'
import { toneOf } from '@/ui/tone'
import styles from './StatusChip.module.css'

export function StatusChip({ status, size = 'small' }: { status: Status; size?: 'small' | 'large' }) {
  return (
    <span
      className={size === 'large' ? `${styles.chip} ${styles.large}` : styles.chip}
      data-tone={toneOf(status)}
    >
      {statusLabel(status)}
    </span>
  )
}
