import { useQuery } from '@tanstack/react-query'
import { useServices } from '@/app/ServicesProvider'
import { queryKeys } from '@/app/queryKeys'

export function useApplication(id: string | undefined) {
  const { applications } = useServices()
  return useQuery({
    queryKey: queryKeys.applications.detail(id ?? ''),
    queryFn: ({ signal }) => applications.get(id as string, { signal, eventLimit: 100 }),
    enabled: Boolean(id),
  })
}
