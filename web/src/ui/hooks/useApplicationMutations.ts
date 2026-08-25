import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useServices } from '@/app/ServicesProvider'
import { queryKeys } from '@/app/queryKeys'
import type { StatusChange } from '@/core/domain/models'

/**
 * A status change invalidates more than the application it touched: the funnel
 * shifts, and a move into a terminal status drops the row out of the follow-up
 * queue. Invalidating both roots is cheaper to reason about than enumerating
 * which derived view happened to change.
 */
function useInvalidatePipeline() {
  const queryClient = useQueryClient()
  return async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.applications.all() }),
      queryClient.invalidateQueries({ queryKey: queryKeys.stats.all() }),
    ])
  }
}

export function useUpdateStatus() {
  const { applications } = useServices()
  const invalidate = useInvalidatePipeline()

  return useMutation({
    mutationFn: ({ id, change }: { id: string; change: StatusChange }) =>
      applications.updateStatus(id, change),
    onSuccess: invalidate,
  })
}

export function useAddNote() {
  const { applications } = useServices()
  const invalidate = useInvalidatePipeline()

  return useMutation({
    mutationFn: ({ id, note }: { id: string; note: string }) => applications.addNote(id, note),
    onSuccess: invalidate,
  })
}
