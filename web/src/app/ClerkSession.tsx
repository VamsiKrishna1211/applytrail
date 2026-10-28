import { ClerkProvider, SignIn, useAuth, useClerk, useUser } from '@clerk/react'
import { createContext, useContext, useEffect, useMemo, type CSSProperties, type ReactNode } from 'react'
import type { ClerkIdentity } from '@/infra/clerk/ClerkIdentity'

/**
 * Clerk sign-in, for a server that offers it.
 *
 * This is the only module that imports Clerk's React SDK. Everything else
 * reads ClerkState, which reads the same whether Clerk is configured or not,
 * so no component has to know which SDK sits behind a sign-in.
 */

export interface ClerkSetup {
  publishableKey: string
  /** The transport's view of the same session; handed Clerk once it loads. */
  identity: ClerkIdentity
}

export interface ClerkState {
  /** This server signs people in with Clerk. */
  enabled: boolean
  /** Clerk knows whether someone is signed in. Always true when not enabled. */
  loaded: boolean
  /** The signed-in Clerk user, if any. */
  userId: string | undefined
  /** How to show who is signed in: their email address. */
  label: string | undefined
  signOut(): void
}

const OFF: ClerkState = {
  enabled: false,
  loaded: true,
  userId: undefined,
  label: undefined,
  signOut: () => {},
}

const ClerkStateContext = createContext<ClerkState>(OFF)

export function useClerkState(): ClerkState {
  return useContext(ClerkStateContext)
}

export function ClerkSession({ clerk, children }: { clerk: ClerkSetup | undefined; children: ReactNode }) {
  if (!clerk) return <>{children}</>
  return (
    <ClerkProvider publishableKey={clerk.publishableKey} afterSignOutUrl="/">
      <ClerkStateBridge identity={clerk.identity}>{children}</ClerkStateBridge>
    </ClerkProvider>
  )
}

function ClerkStateBridge({ identity, children }: { identity: ClerkIdentity; children: ReactNode }) {
  const clerk = useClerk()
  const { isLoaded, userId, signOut } = useAuth()
  const { user } = useUser()

  useEffect(() => {
    if (isLoaded) identity.attach(clerk)
  }, [clerk, identity, isLoaded])

  const state = useMemo<ClerkState>(
    () => ({
      enabled: true,
      loaded: isLoaded,
      userId: userId ?? undefined,
      label: user?.primaryEmailAddress?.emailAddress ?? undefined,
      signOut: () => void signOut(),
    }),
    [isLoaded, signOut, user, userId],
  )

  return <ClerkStateContext.Provider value={state}>{children}</ClerkStateContext.Provider>
}

/**
 * Clerk defines its own --accent inside its components, which shadows the
 * app's token and turns var(--accent) there into a circular reference. The
 * wrapper resolves the app's accent first, under a name Clerk does not use,
 * and Clerk's elements inherit the resolved colour — so a theme switch still
 * reaches the form.
 */
const brand = { '--applytrail-accent': 'var(--accent)' } as CSSProperties

/**
 * Clerk's sign-in form, dressed in the app's own tokens.
 *
 * Hash routing keeps the form's steps (email, then code) out of the app's
 * routes: the gate renders before the router has anything to show.
 */
export function ClerkSignIn() {
  return (
    <div style={brand}>
      <SignIn
        routing="hash"
        appearance={{
          variables: {
            colorPrimary: 'var(--applytrail-accent)',
            colorPrimaryForeground: 'var(--surface)',
            colorBackground: 'var(--surface)',
            colorForeground: 'var(--ink)',
            colorMutedForeground: 'var(--muted)',
            colorInput: 'var(--surface)',
            colorInputForeground: 'var(--ink)',
            colorDanger: 'var(--stop)',
            colorNeutral: 'var(--ink)',
            fontFamily: 'inherit',
            borderRadius: '8px',
          },
        }}
      />
    </div>
  )
}
