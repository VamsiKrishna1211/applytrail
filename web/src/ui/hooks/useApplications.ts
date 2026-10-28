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

  // An explicitly empty status array means "match nothing" to the caller,
  // but the wire format cannot express that: `status` repeats as a query
  // parameter per value, so zero values serialises to no parameter at all —
  // indistinguishable from "no filter", which the server reads as "every
  // status". Short-circuited in the query function itself, not via
  // `enabled: false`: a disabled query never leaves `isPending`, which would
  // trade "silently fetches everything" for "silently loads forever." This
  // way it resolves normally, immediately, to a real empty page.
  const wouldMatchNothing = query.status !== undefined && query.status.length === 0

  const result = useInfiniteQuery({
    queryKey: queryKeys.applications.list(query),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      wouldMatchNothing
        ? Promise.resolve({ items: [], nextCursor: undefined })
        : applications.list({ ...query, ...(pageParam ? { cursor: pageParam } : {}) }, { signal }),
    getNextPageParam: (lastPage) => lastPage.nextCursor,
    enabled,
  })

  return {
    ...result,
    applications: result.data?.pages.flatMap((page) => page.items) ?? [],
  }
}
