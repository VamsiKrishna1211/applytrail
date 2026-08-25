import { createContext, useContext, useMemo, useSyncExternalStore, type ReactNode } from 'react'
import type { AuthRejection } from '@/core/ports/authEvents'
import type { Services } from './services'

const ServicesContext = createContext<Services | undefined>(undefined)

export function ServicesProvider({
  services,
  children,
}: {
  services: Services
  children: ReactNode
}) {
  return <ServicesContext.Provider value={services}>{children}</ServicesContext.Provider>
}

export function useServices(): Services {
  const services = useContext(ServicesContext)
  if (!services) {
    throw new Error('useServices must be used inside a ServicesProvider')
  }
  return services
}

export interface Credential {
  token: string | undefined
  unlocked: boolean
  /** The last four characters, for display. Never the whole key. */
  fingerprint: string
  /** Why the last credential was refused, if one was. Cleared when a new key is offered. */
  rejection: AuthRejection | undefined
  set(token: string): void
  clear(): void
}

/**
 * Subscribes to the credential store.
 *
 * useSyncExternalStore rather than state so that a credential dropped by the
 * transport — after a 401, from inside a fetch — re-renders the shell
 * immediately instead of on the next unrelated update.
 */
export function useCredential(): Credential {
  const { credentials, authEvents } = useServices()

  const token = useSyncExternalStore(
    (listener) => credentials.subscribe(listener),
    () => credentials.read(),
    () => undefined,
  )

  const rejection = useSyncExternalStore(
    (listener) => authEvents.subscribe(listener),
    () => authEvents.last(),
    () => undefined,
  )

  return useMemo(
    () => ({
      token,
      unlocked: Boolean(token),
      fingerprint: token ? token.slice(-4) : '',
      rejection,
      set: (next: string) => {
        // Drop the previous reason first: it belongs to the old key, and
        // leaving it up would report a stale failure against a new one.
        authEvents.acknowledge()
        credentials.write(next.trim())
      },
      clear: () => credentials.clear(),
    }),
    [authEvents, credentials, rejection, token],
  )
}
