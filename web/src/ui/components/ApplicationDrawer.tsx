import { useEffect, useState, type FormEvent } from 'react'
import { useServices } from '@/app/ServicesProvider'
import { statusLabel, type Status } from '@/core/domain/status'
import { describeMoves } from '@/core/usecases/describeMoves'
import { useApplication } from '@/ui/hooks/useApplication'
import { useAddNote, useUpdateStatus } from '@/ui/hooks/useApplicationMutations'
import { dwellLabel, formatDate } from '@/ui/format'
import { toneOf } from '@/ui/tone'
import { EmptyState, ErrorNotice, Skeleton } from './Feedback'
import { StatusChip } from './StatusChip'
import { Timeline } from './Timeline'
import drawer from './ApplicationDrawer.module.css'
import moveBar from './MoveBar.module.css'

/**
 * One application, with everything that can be done to it.
 *
 * It lives in the shell rather than on a page because both the dashboard and
 * the board open it, and duplicating it would be two places for the timeline
 * to drift apart.
 */
export function ApplicationDrawer({ id, onClose }: { id: string; onClose: () => void }) {
  const { clock } = useServices()
  const detail = useApplication(id)
  const updateStatus = useUpdateStatus()
  const addNote = useAddNote()
  const [draft, setDraft] = useState('')

  // Escape closes it. Bound while the drawer is open and released on unmount,
  // so nothing lingers on the document once it is gone.
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  const application = detail.data?.application

  function move(to: Status) {
    updateStatus.mutate({ id, change: { status: to } })
  }

  function submitNote(event: FormEvent) {
    event.preventDefault()
    const note = draft.trim()
    if (!note) return
    addNote.mutate({ id, note }, { onSuccess: () => setDraft('') })
  }

  return (
    <>
      <button type="button" className={drawer.scrim} aria-label="Close" onClick={onClose} />

      <aside className={drawer.drawer} aria-label="Application detail">
        <header className={drawer.head}>
          <div className={drawer.headTop}>
            <span className="eyebrow">Application</span>
            <span className={drawer.id}>{id.slice(0, 8)}</span>
            <button type="button" className={drawer.close} onClick={onClose} aria-label="Close">
              ×
            </button>
          </div>

          {detail.isPending ? (
            <Skeleton height={96} />
          ) : detail.isError ? (
            <ErrorNotice error={detail.error} what="load this application" />
          ) : application ? (
            <>
              <h2 className={drawer.title}>{application.title ?? 'untitled posting'}</h2>
              <p className={drawer.subtitle}>
                {application.company ?? 'unknown company'}
                {application.location ? ` · ${application.location}` : ''}
              </p>
              {application.url ? (
                <a
                  className={drawer.link}
                  href={application.url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  {application.url}
                </a>
              ) : null}

              <div className={drawer.facts}>
                <StatusChip status={application.status} size="large" />
                <span>applied {formatDate(application.applied_at)}</span>
                <span>{dwellLabel(application.updated_at, clock)}</span>
                {application.next_action_at ? (
                  <span>next action {formatDate(application.next_action_at)}</span>
                ) : null}
              </div>
            </>
          ) : null}
        </header>

        {application ? (
          <div className={drawer.section}>
            <h3 className="eyebrow">Move to</h3>
            <div className={moveBar.moves} style={{ marginTop: '.6rem' }}>
              {describeMoves(application.status).moves.map((option) => (
                <button
                  key={option.to}
                  type="button"
                  className={moveBar.chip}
                  data-tone={toneOf(option.to)}
                  disabled={updateStatus.isPending}
                  onClick={() => move(option.to)}
                >
                  {option.label}
                </button>
              ))}
              {describeMoves(application.status).terminal ? (
                <span className={moveBar.terminal}>
                  Terminal. Every transition from {statusLabel(application.status)} is rejected.
                </span>
              ) : null}
            </div>
            <p className={moveBar.hint}>{describeMoves(application.status).hint}</p>
            {updateStatus.error ? (
              <div className={moveBar.failure}>
                <ErrorNotice error={updateStatus.error} what="move this application" />
              </div>
            ) : null}
          </div>
        ) : null}

        <div className={drawer.scroll}>
          {application?.notes ? (
            <>
              <div className={drawer.sectionHead}>
                <h3 className="eyebrow">Notes</h3>
                <span className={drawer.count}>accumulated, newest last</span>
              </div>
              <p className={drawer.notes}>{application.notes}</p>
            </>
          ) : null}

          <div className={drawer.sectionHead}>
            <h3 className="eyebrow">Timeline</h3>
            <span className={drawer.count}>
              append-only · {detail.data?.events.length ?? 0} events
            </span>
          </div>

          {detail.isPending ? (
            <Skeleton height={200} />
          ) : detail.data && detail.data.events.length > 0 ? (
            <Timeline events={detail.data.events} />
          ) : (
            <EmptyState>No events recorded yet.</EmptyState>
          )}
        </div>

        <form className={drawer.composer} onSubmit={submitNote}>
          <label className="eyebrow" htmlFor="note-draft">
            Append a note
          </label>
          <textarea
            id="note-draft"
            className={drawer.textarea}
            rows={2}
            value={draft}
            placeholder="Recruiter said the loop is being scheduled for next week…"
            onChange={(event) => setDraft(event.target.value)}
          />
          <div className={drawer.composerRow}>
            <button type="submit" className={drawer.submit} disabled={!draft.trim() || addNote.isPending}>
              {addNote.isPending ? 'Appending…' : 'Append note'}
            </button>
            <span className={drawer.composerNote}>Notes accumulate; nothing overwrites.</span>
          </div>
          {addNote.error ? (
            <div className={moveBar.failure}>
              <ErrorNotice error={addNote.error} what="append the note" />
            </div>
          ) : null}
        </form>
      </aside>
    </>
  )
}
