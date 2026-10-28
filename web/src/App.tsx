import { QueryClientProvider, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ClerkSession, useClerkState, type ClerkSetup } from '@/app/ClerkSession'
import { ServicesProvider, useCredential } from '@/app/ServicesProvider'
import { createQueryClient } from '@/app/queryClient'
import type { Services } from '@/app/services'
import { AppShell } from '@/ui/components/AppShell'
import { ErrorBoundary } from '@/ui/components/ErrorBoundary'
import { DashboardPage } from '@/ui/routes/DashboardPage'
import { PipelinePage } from '@/ui/routes/PipelinePage'
import { UnlockPage } from '@/ui/routes/UnlockPage'

/**
 * The app takes its services as a prop.
 *
 * That is what makes the whole tree testable and what keeps `createServices`
 * out of every module: main.tsx builds the real graph, a test builds a stub
 * one, and nothing in between knows the difference.
 */
export function App({ services, clerk }: { services: Services; clerk?: ClerkSetup }) {
  const queryClient = useMemo(() => createQueryClient(), [])

  return (
    <ServicesProvider services={services}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <ErrorBoundary>
            <ClerkSession clerk={clerk}>
              <Shell />
            </ClerkSession>
          </ErrorBoundary>
        </BrowserRouter>
      </QueryClientProvider>
    </ServicesProvider>
  )
}

function Shell() {
  const credential = useCredential()
  const clerk = useClerkState()
  const queryClient = useQueryClient()

  // Who the requests are made as: the pasted key when there is one (it wins
  // in the transport too), otherwise the signed-in Clerk user.
  const identity = credential.token ?? clerk.userId

  // A different identity — including "→ none" on sign-out, and "none → key"
  // on unlock — means every cached application, stat, and follow-up in
  // memory belongs to someone who is no longer the one in use. Without this,
  // unlocking with a second key rendered the first key's pipeline from cache
  // (staleTime is 30s) until the first fresh request landed: a multi-tenant
  // data leak across sessions in the same tab. The 401 path (services.ts's
  // onAuthFailure dropping the key or signing out of Clerk) lands here too,
  // since both stores it touches are ones this component subscribes to.
  //
  // The cache is cleared with nothing mounted. Clearing it from an effect
  // while the new identity's pages were already rendering cancelled the
  // queries they had just started — child effects run before a parent's —
  // and nothing restarted them, so a page reloaded while signed in to Clerk
  // sat on empty panels indefinitely. Rendering nothing for the one frame in
  // which the cache still belongs to someone else means every query starts
  // against a cache that is already clean.
  const [cacheOwner, setCacheOwner] = useState(identity)
  useEffect(() => {
    if (cacheOwner !== identity) {
      queryClient.clear()
      setCacheOwner(identity)
    }
  }, [cacheOwner, identity, queryClient])
  if (cacheOwner !== identity) return null

  // Until Clerk has said whether someone is signed in, render nothing rather
  // than flash the sign-in form at a person who is already signed in.
  if (!identity && !clerk.loaded) return null

  // Without a credential every request is a 401, so the gate comes before the
  // router rather than as a redirect: there is no route worth rendering yet.
  if (!identity) return <UnlockPage />

  return (
    <Routes>
      <Route element={<AppShell wide fill />}>
        <Route index element={<DashboardPage />} />
        <Route path="pipeline" element={<PipelinePage />} />
        {/* An unknown path stays on its own URL rather than silently
            rendering the dashboard under a URL that doesn't say so — no
            route in this app produces a link to anything else, so this is
            reachable only by a stale bookmark or a typed URL. */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
