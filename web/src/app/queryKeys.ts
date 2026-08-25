import type { ListApplicationsQuery } from '@/core/domain/models'

/**
 * Every cache key in one place, so invalidation after a mutation is a fact
 * about this file rather than a string repeated at each call site.
 *
 * Keys are nested: invalidating `applications.all()` reaches every list, every
 * detail and the follow-up queue, which is exactly what a status change means.
 */
export const queryKeys = {
  applications: {
    all: () => ['applications'] as const,
    list: (query: ListApplicationsQuery) => ['applications', 'list', query] as const,
    detail: (id: string) => ['applications', 'detail', id] as const,
    followups: (dueBefore?: string) => ['applications', 'followups', dueBefore ?? null] as const,
  },
  stats: {
    all: () => ['stats'] as const,
  },
} as const
