import { QueryClient } from '@tanstack/react-query'
import { isApiError } from '@/infra/http/ApiError'

/**
 * Retry only what retrying could fix.
 *
 * A 400 means the request was wrong and will be wrong again; a 401 means the
 * credential is gone. Repeating either burns the caller's rate-limit budget to
 * arrive at the same answer, and the auth-failure budget is service-wide.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: true,
        retry: (failureCount, error) => {
          if (isApiError(error) && !error.isRetryable) return false
          return failureCount < 2
        },
      },
      mutations: {
        retry: false,
      },
    },
  })
}
