import { useQuery } from '@tanstack/react-query'
import { useServices } from '@/app/ServicesProvider'
import { queryKeys } from '@/app/queryKeys'
import { byUrgency } from '@/core/usecases/followupUrgency'

export function useFollowups(limit = 50, enabled = true) {
  const { applications } = useServices()
  return useQuery({
    queryKey: queryKeys.applications.followups(),
    queryFn: ({ signal }) => applications.followups({ limit }, { signal }),
    // The server orders by its own rule; the UI wants most-overdue first and
    // sorts a copy so the cached array is never mutated in place.
    select: (followups) => [...followups].sort(byUrgency),
    enabled,
  })
}
