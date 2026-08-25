import { useId, useState, type FormEvent } from 'react'
import { useCredential } from '@/app/ServicesProvider'
import styles from './UnlockPage.module.css'

/**
 * The credential gate.
 *
 * There is no browser login: the API authenticates with the same key an agent
 * uses, and this screen is where a person hands one over. The key is held for
 * the life of the tab and never written to localStorage — see
 * SessionCredentialStore for why, and for what that still does not protect
 * against.
 */
export function UnlockPage() {
  const credential = useCredential()
  const [value, setValue] = useState('')
  const fieldId = useId()

  function submit(event: FormEvent) {
    event.preventDefault()
    const token = value.trim()
    if (token) credential.set(token)
  }

  return (
    <div className={styles.page}>
      <form className={styles.card} onSubmit={submit}>
        <div className={styles.brand}>
          <span className={styles.mark} aria-hidden="true" />
          ApplyTrail
        </div>

        <h1 className={styles.title}>Unlock this session</h1>
        <p className={styles.lede}>
          Paste an API key. It stays in this tab and is gone when you close it.
        </p>

        {credential.rejection ? (
          <div className={styles.rejected} role="alert">
            <div className={styles.rejectedHead}>
              <strong>That key was refused.</strong>
              <span className={styles.rejectedCode}>{credential.rejection.code}</span>
            </div>
            <div>{credential.rejection.message}</div>
            <p className={styles.rejectedHint}>
              A key exists only in the database it was created in. If this server was started
              against a different one, a perfectly valid key still will not authenticate here.
            </p>
            {credential.rejection.requestId ? (
              <div className={styles.rejectedId}>request {credential.rejection.requestId}</div>
            ) : null}
          </div>
        ) : null}

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
      </form>
    </div>
  )
}
