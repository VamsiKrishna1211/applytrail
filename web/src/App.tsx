import { QueryClientProvider } from '@tanstack/react-query'
import { useMemo } from 'react'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { ServicesProvider, useCredential } from '@/app/ServicesProvider'
import { createQueryClient } from '@/app/queryClient'
import type { Services } from '@/app/services'
import { AppShell } from '@/ui/components/AppShell'
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
export function App({ services }: { services: Services }) {
  const queryClient = useMemo(() => createQueryClient(), [])

  return (
    <ServicesProvider services={services}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Shell />
        </BrowserRouter>
      </QueryClientProvider>
    </ServicesProvider>
  )
}

function Shell() {
  const credential = useCredential()

  // Without a credential every request is a 401, so the gate comes before the
  // router rather than as a redirect: there is no route worth rendering yet.
  if (!credential.unlocked) return <UnlockPage />

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
      </Route>
      <Route element={<AppShell wide />}>
        <Route path="pipeline" element={<PipelinePage />} />
      </Route>
      <Route path="*" element={<AppShell />}>
        <Route path="*" element={<DashboardPage />} />
      </Route>
    </Routes>
  )
}
