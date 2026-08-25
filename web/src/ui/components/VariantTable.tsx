import { Fragment } from 'react'
import type { ResumeVariantStats } from '@/core/domain/models'
import { responseRateRatio } from '@/core/usecases/buildFunnel'
import styles from './VariantTable.module.css'

const COLUMNS = ['Sent', 'Replied', 'Rejected', 'Ghosted'] as const

/** Which resume actually gets answered — the one number in the app worth changing behaviour over. */
export function VariantTable({ variants }: { variants: readonly ResumeVariantStats[] }) {
  return (
    <div className={styles.table}>
      <div className={styles.head}>Variant</div>
      {COLUMNS.map((column) => (
        <div key={column} className={`${styles.head} ${styles.numHead}`}>
          {column}
        </div>
      ))}
      <div className={styles.head}>Rate</div>

      {variants.map((variant) => (
        <Fragment key={variant.variant}>
          <div className={`${styles.cell} ${styles.name}`}>{variant.variant || 'unlabelled'}</div>
          <div className={`${styles.cell} ${styles.num}`}>{variant.sent}</div>
          <div className={`${styles.cell} ${styles.num}`}>{variant.responded}</div>
          <div className={`${styles.cell} ${styles.num}`}>{variant.rejected}</div>
          <div className={`${styles.cell} ${styles.num}`}>{variant.ghosted}</div>
          <div className={styles.rate}>
            <span className={styles.track}>
              <span
                className={styles.bar}
                style={{ width: `${Math.round(responseRateRatio(variant.response_rate) * 100)}%` }}
              />
            </span>
            <span className={styles.pct}>{Math.round(variant.response_rate * 100)}%</span>
          </div>
        </Fragment>
      ))}
    </div>
  )
}
