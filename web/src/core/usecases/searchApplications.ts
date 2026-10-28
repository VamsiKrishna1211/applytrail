import type { Application } from '@/core/domain/models'
import { statusLabel } from '@/core/domain/status'

/**
 * Free-text filtering of a loaded board.
 *
 * The API's only text parameter is an exact company match, so this filters
 * applications the caller already holds rather than issuing a query: a
 * pipeline is at most a few hundred rows, and a filter that runs as you type
 * is worth more than one that is authoritative over rows nobody has fetched.
 * The caller is responsible for saying so when pages remain — see GlobalSearch.
 */

/** Every field the UI puts on screen, so what matches is what can be read. */
function haystack(application: Application): string {
  return [
    application.company,
    application.title,
    application.location,
    application.resume_variant,
    application.cover_letter_variant,
    application.notes,
    statusLabel(application.status),
    // The posting-link commit put the URL on screen (MoveBar, the drawer);
    // this was the one field that landed there without landing here too, so
    // searching by job-board host or requisition id silently matched nothing.
    application.url,
  ]
    .filter(Boolean)
    .join('\n')
    .toLowerCase()
}

/** Whitespace-separated words, lowercased. Empty input means no terms, which matches everything. */
export function searchTerms(text: string): string[] {
  return text.toLowerCase().split(/\s+/).filter(Boolean)
}

/**
 * Every term must appear somewhere, in any order and any field: "goog staff"
 * finds a staff role at Google. Substrings match, so a half-typed company
 * narrows the board before it is finished.
 */
export function matchesSearch(application: Application, terms: readonly string[]): boolean {
  if (terms.length === 0) return true
  const text = haystack(application)
  return terms.every((term) => text.includes(term))
}

export function searchApplications(
  applications: readonly Application[],
  text: string,
): Application[] {
  const terms = searchTerms(text)
  if (terms.length === 0) return [...applications]
  return applications.filter((application) => matchesSearch(application, terms))
}
