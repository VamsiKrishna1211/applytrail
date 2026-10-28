import { useId, useState, type FormEvent, type ReactNode } from 'react'
import { ClerkSignIn, useClerkState } from '@/app/ClerkSession'
import { useCredential, type Credential } from '@/app/ServicesProvider'
import styles from './UnlockPage.module.css'

/**
 * The credential gate.
 *
 * On a server that offers Clerk, a person signs in with their Clerk account
 * and the session mints its own short-lived tokens. Every server also accepts
 * the same API key an agent uses, so pasting one stays possible behind a
 * switch. A pasted key is held for the life of the tab and never written to
 * localStorage — see SessionCredentialStore for why, and for what that still
 * does not protect against.
 */
export function UnlockPage() {
  const clerk = useClerkState()
  const credential = useCredential()
  const [withKey, setWithKey] = useState(!clerk.enabled)

  const rejection = credential.rejection ? <Rejection credential={credential} /> : null

  if (clerk.enabled && !withKey) {
    return (
      <div className={styles.page}>
        <div className={styles.stack}>
          <div className={styles.brand}>
            <span className={styles.mark} aria-hidden="true" />
            ApplyTrail
          </div>
          {rejection}
          <ClerkSignIn />
          <button type="button" className={styles.switch} onClick={() => setWithKey(true)}>
            Use an API key instead
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <KeyForm rejection={rejection} credential={credential}>
        {clerk.enabled ? (
          <button type="button" className={styles.switch} onClick={() => setWithKey(false)}>
            Sign in with your account instead
          </button>
        ) : null}
      </KeyForm>
    </div>
  )
}

function KeyForm({
  credential,
  rejection,
  children,
}: {
  credential: Credential
  rejection: ReactNode
  children: ReactNode
}) {
  const [value, setValue] = useState('')
  const fieldId = useId()

  function submit(event: FormEvent) {
    event.preventDefault()
    const token = value.trim()
    if (token) credential.set(token)
  }

  return (
    <form className={styles.card} onSubmit={submit}>
      <div className={styles.brand}>
        <span className={styles.mark} aria-hidden="true" />
        ApplyTrail
      </div>

      <h1 className={styles.title}>Unlock this session</h1>
      <p className={styles.lede}>
        Paste an API key. It stays in this tab and is gone when you close it.
      </p>

      {rejection}

      <label className={styles.field} htmlFor={fieldId}>
        <span className={`eyebrow ${styles.label}`}>API key</span>
        <input
          id={fieldId}
          className={styles.input}
          type="password"
          autoComplete="off"
          spellCheck={false}
          placeholder="at_live_…"
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </label>

      <button type="submit" className={styles.submit} disabled={!value.trim()}>
        Unlock
      </button>

      <p className={styles.note}>
        Give this UI a key with <code>read</code> and <code>write</code> scope, not{' '}
        <code>admin</code>. Anything that can run script on this page can read the key it is
        holding, and an admin key can mint more credentials.
      </p>

      {children}
    </form>
  )
}

/** Why the last credential was refused, worded for the kind it was. */
function Rejection({ credential }: { credential: Credential }) {
  const rejection = credential.rejection
  if (!rejection) return null
  const signIn = rejection.credential === 'sign_in'

  return (
    <div className={styles.rejected} role="alert">
      <div className={styles.rejectedHead}>
        <strong>{signIn ? 'That account was refused.' : 'That key was refused.'}</strong>
        <span className={styles.rejectedCode}>{rejection.code}</span>
      </div>
      <div>{rejection.message}</div>
      <p className={styles.rejectedHint}>
        {signIn
          ? 'Signing in worked, but this server does not link that account to any ApplyTrail data. Its operator adds accounts to APPLYTRAIL_CLERK_TENANTS.'
          : 'A key exists only in the database it was created in. If this server was started against a different one, a perfectly valid key still will not authenticate here.'}
      </p>
      {rejection.requestId ? (
        <div className={styles.rejectedId}>request {rejection.requestId}</div>
      ) : null}
    </div>
  )
}
