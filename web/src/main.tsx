import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { createServices } from './app/services'
import { ClerkIdentity } from './infra/clerk/ClerkIdentity'
import { loadWebConfig } from './infra/http/loadWebConfig'
import './styles/base.css'

const root = document.getElementById('root')
if (!root) throw new Error('#root is missing from index.html')

const baseUrl = import.meta.env.VITE_API_BASE_URL ?? ''

// How this server signs people in decides what the gate shows, so it is read
// before the first render rather than flashing an API key form that is about
// to be replaced.
void loadWebConfig(baseUrl).then(({ clerkPublishableKey }) => {
  const identity = clerkPublishableKey ? new ClerkIdentity() : undefined

  createRoot(root).render(
    <StrictMode>
      <App
        services={createServices({ baseUrl, ...(identity ? { identity } : {}) })}
        {...(clerkPublishableKey && identity
          ? { clerk: { publishableKey: clerkPublishableKey, identity } }
          : {})}
      />
    </StrictMode>,
  )
})
