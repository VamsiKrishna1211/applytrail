import { isApiError } from '@/infra/http/ApiError'
import styles from './Feedback.module.css'

export function EmptyState({ children }: { children: React.ReactNode }) {
  return <p className={styles.empty}>{children}</p>
}

/**
 * Failure, reported the way the server reported it.
 *
 * `details` is the field-level list the API sends in one pass — on a rejected
 * transition it names the statuses that would have been accepted, which is the
 * most useful sentence on the screen. The request id is shown because it is
 * what matches the server's logs.
 */
export function ErrorNotice({ error, what }: { error: unknown; what: string }) {
  const api = isApiError(error) ? error : undefined
  const message = api?.message ?? (error instanceof Error ? error.message : 'unknown error')

  return (
    <div className={styles.error} role="alert">
      <div className={styles.errorHead}>
        <strong>Could not {what}.</strong>
        {api ? <span className={styles.code}>{api.code}</span> : null}
      </div>
      <div>{message}</div>
      {api && api.details.length > 0 ? (
        <ul className={styles.details}>
          {api.details.map((detail, index) => (
            // Not key={detail}: the API can legitimately repeat the same
            // problem string (details is a flat list of field-level messages,
            // not a set), which collided on the text as a key.
            <li key={index}>{detail}</li>
          ))}
        </ul>
      ) : null}
      {api?.requestId ? <div className={styles.requestId}>request {api.requestId}</div> : null}
    </div>
  )
}

export function Skeleton({ height, width = '100%' }: { height: number | string; width?: number | string }) {
  return <div className={styles.skeleton} style={{ height, width }} aria-hidden="true" />
}
