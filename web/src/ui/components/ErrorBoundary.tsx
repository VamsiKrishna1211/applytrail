import { Component, type ErrorInfo, type ReactNode } from 'react'
import styles from './Feedback.module.css'

/**
 * The last line of defence against an uncaught render throw.
 *
 * React Query already catches a thrown mapping error (mapping.ts) inside a
 * query or mutation function and turns it into `error` state, which every
 * screen already renders through ErrorNotice — this boundary is not that
 * path. It exists for whatever that safety net does not cover: a render-time
 * throw from anywhere else in the tree, present or future. Without it, one
 * unrecognised value anywhere goes from "one broken panel" to "a blank white
 * page," which is a worse failure for no benefit.
 */
export class ErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | undefined }
> {
  override state: { error: Error | undefined } = { error: undefined }

  static getDerivedStateFromError(error: unknown): { error: Error } {
    return { error: error instanceof Error ? error : new Error(String(error)) }
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('unhandled render error', error, info.componentStack)
  }

  override render() {
    if (this.state.error) {
      return (
        <div className={styles.error} role="alert">
          <div className={styles.errorHead}>
            <strong>Something went wrong rendering this page.</strong>
          </div>
          <div>{this.state.error.message}</div>
          <div>
            <button type="button" onClick={() => location.reload()}>
              Reload
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
