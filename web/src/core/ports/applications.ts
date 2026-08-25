import type {
  Application,
  ApplicationDetail,
  FollowUp,
  ListApplicationsQuery,
  Page,
  StatusChange,
} from '@/core/domain/models'

export interface ReadOptions {
  signal?: AbortSignal
}

/**
 * Reading the pipeline. Split from writing so a component that only renders
 * cannot be handed the ability to mutate, and so a read-only credential maps
 * onto a genuinely read-only dependency.
 */
export interface ApplicationsReader {
  list(query: ListApplicationsQuery, options?: ReadOptions): Promise<Page<Application>>
  get(id: string, options?: ReadOptions & { eventLimit?: number }): Promise<ApplicationDetail>
  followups(
    query?: { dueBefore?: string; limit?: number },
    options?: ReadOptions,
  ): Promise<FollowUp[]>
}

export interface ApplicationsWriter {
  updateStatus(id: string, change: StatusChange): Promise<Application>
  addNote(id: string, note: string): Promise<Application>
}
