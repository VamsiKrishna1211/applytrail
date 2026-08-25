import type { Application } from '@/core/domain/models'
import type { Status } from '@/core/domain/status'

export interface BoardColumn {
  status: Status
  applications: Application[]
}

/**
 * Buckets applications into the given columns, preserving the order the server
 * returned them in.
 *
 * Applications whose status is outside `columns` are dropped rather than piled
 * into a catch-all: the Active and Closed views each own a complete set of
 * statuses, and a card with nowhere to go means the sets have drifted.
 */
export function buildBoard(applications: readonly Application[], columns: readonly Status[]): BoardColumn[] {
  const byStatus = new Map<Status, Application[]>(columns.map((status) => [status, []]))
  for (const application of applications) {
    byStatus.get(application.status)?.push(application)
  }
  return columns.map((status) => ({ status, applications: byStatus.get(status) ?? [] }))
}
