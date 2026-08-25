import type { Clock } from '@/core/ports/clock'
import type { ApplicationsReader, ApplicationsWriter } from '@/core/ports/applications'
import type { AuthEvents } from '@/core/ports/authEvents'
import type { CredentialStore } from '@/core/ports/credentials'
import type { StatsReader } from '@/core/ports/stats'
import { InMemoryAuthEvents } from '@/infra/InMemoryAuthEvents'
import { SystemClock } from '@/infra/SystemClock'
import { FetchTransport } from '@/infra/http/FetchTransport'
import { HttpApplicationsRepository } from '@/infra/repositories/HttpApplicationsRepository'
import { HttpStatsRepository } from '@/infra/repositories/HttpStatsRepository'
import { SessionCredentialStore } from '@/infra/storage/SessionCredentialStore'

/**
 * Everything the UI is allowed to depend on, as interfaces.
 *
 * Components resolve these from context and never construct one, which is what
 * keeps `new HttpApplicationsRepository(...)` confined to the function below.
 */
export interface Services {
  credentials: CredentialStore
  authEvents: AuthEvents
  applications: ApplicationsReader & ApplicationsWriter
  stats: StatsReader
  clock: Clock
}

export interface ServiceOptions {
  /** API root. Empty means same-origin, which is what the embedded build serves. */
  baseUrl?: string
  credentials?: CredentialStore
  authEvents?: AuthEvents
  clock?: Clock
  fetch?: typeof globalThis.fetch
}

/**
 * The composition root. The only function that names a concrete class.
 *
 * The transport reads the token through the store on every call rather than
 * capturing it, so unlocking, re-keying and signing out take effect on the
 * next request without anything being rebuilt.
 */
export function createServices(options: ServiceOptions = {}): Services {
  const credentials = options.credentials ?? new SessionCredentialStore()
  const authEvents = options.authEvents ?? new InMemoryAuthEvents()

  const transport = new FetchTransport({
    baseUrl: options.baseUrl ?? import.meta.env.VITE_API_BASE_URL ?? '',
    token: () => credentials.read(),
    // A credential the server rejects is worse than none: it leaves the shell
    // showing a signed-in UI that fails every request. Drop it — but record why
    // first, so the unlock screen can explain itself instead of just
    // reappearing.
    onAuthFailure: (error) => {
      authEvents.reject({
        code: error.code,
        message: error.message,
        requestId: error.requestId,
      })
      credentials.clear()
    },
    ...(options.fetch ? { fetch: options.fetch } : {}),
  })

  return {
    credentials,
    authEvents,
    applications: new HttpApplicationsRepository(transport),
    stats: new HttpStatsRepository(transport),
    clock: options.clock ?? new SystemClock(),
  }
}
