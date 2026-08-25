import { useQuery } from '@tanstack/react-query'
import { useServices } from '@/app/ServicesProvider'
import { queryKeys } from '@/app/queryKeys'

export function useStats(enabled = true) {
  const { stats } = useServices()
  return useQuery({
    queryKey: queryKeys.stats.all(),
    queryFn: ({ signal }) => stats.get({ signal }),
    enabled,
  })
}
