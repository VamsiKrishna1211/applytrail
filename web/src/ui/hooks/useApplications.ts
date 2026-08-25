import { useInfiniteQuery } from '@tanstack/react-query'
import { useServices } from '@/app/ServicesProvider'
import { queryKeys } from '@/app/queryKeys'
import type { ListApplicationsQuery } from '@/core/domain/models'

/**
 * A cursor-paged list, flattened.
 *
 * The board wants one array; the API pages. Infinite query keeps the two
 * reconciled without the board learning what a cursor is, and `hasNextPage`
 * is what the "load more" control binds to — the alternative, silently
 * fetching until exhaustion, hides an unbounded number of requests behind a
 * render.
 */
export function useApplications(query: ListApplicationsQuery, enabled = true) {
  const { applications } = useServices()

  const result = useInfiniteQuery({
    queryKey: queryKeys.applications.list(query),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      applications.list({ ...query, ...(pageParam ? { cursor: pageParam } : {}) }, { signal }),
    getNextPageParam: (lastPage) => lastPage.nextCursor,
    enabled,
  })

  return {
    ...result,
    applications: result.data?.pages.flatMap((page) => page.items) ?? [],
  }
}
