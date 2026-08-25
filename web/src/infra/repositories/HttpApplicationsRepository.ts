import type {
  Application,
  ApplicationDetail,
  FollowUp,
  ListApplicationsQuery,
  Page,
  StatusChange,
} from '@/core/domain/models'
import type { ApplicationsReader, ApplicationsWriter, ReadOptions } from '@/core/ports/applications'
import type { Transport } from '@/core/ports/transport'
import { toApplication, toDetail, toFollowUp } from './mapping'

interface ListResponse {
  applications?: unknown[]
  next_cursor?: string
}

interface FollowupsResponse {
  followups?: unknown[]
}

/**
 * Applications over the REST API.
 *
 * It knows paths, parameter names and response envelopes; it does not know how
 * a request is sent or how a failure is presented. Adding contacts or search
 * profiles means a sibling class, not an edit here.
 */
export class HttpApplicationsRepository implements ApplicationsReader, ApplicationsWriter {
  readonly #transport: Transport

  constructor(transport: Transport) {
    this.#transport = transport
  }

  async list(query: ListApplicationsQuery, options?: ReadOptions): Promise<Page<Application>> {
    const response = await this.#transport.send<ListResponse>({
      method: 'GET',
      path: '/v1/applications',
      query: {
        status: query.status,
        company: query.company,
        since: query.since,
        limit: query.limit,
        cursor: query.cursor,
      },
      ...(options?.signal ? { signal: options.signal } : {}),
    })
    return {
      items: (response.applications ?? []).map(toApplication),
      // The server omits the cursor on the last page; an empty string is the
      // end, not a cursor that returns everything again.
      nextCursor: response.next_cursor || undefined,
    }
  }

  async get(
    id: string,
    options?: ReadOptions & { eventLimit?: number },
  ): Promise<ApplicationDetail> {
    const response = await this.#transport.send<unknown>({
      method: 'GET',
      path: `/v1/applications/${encodeURIComponent(id)}`,
      query: { event_limit: options?.eventLimit },
      ...(options?.signal ? { signal: options.signal } : {}),
    })
    return toDetail(response)
  }

  async followups(
    query?: { dueBefore?: string; limit?: number },
    options?: ReadOptions,
  ): Promise<FollowUp[]> {
    const response = await this.#transport.send<FollowupsResponse>({
      method: 'GET',
      path: '/v1/applications/followups',
      query: { due_before: query?.dueBefore, limit: query?.limit },
      ...(options?.signal ? { signal: options.signal } : {}),
    })
    return (response.followups ?? []).map(toFollowUp)
  }

  async updateStatus(id: string, change: StatusChange): Promise<Application> {
    const response = await this.#transport.send<unknown>({
      method: 'PATCH',
      path: `/v1/applications/${encodeURIComponent(id)}`,
      body: change,
    })
    return toApplication(response)
  }

  async addNote(id: string, note: string): Promise<Application> {
    const response = await this.#transport.send<unknown>({
      method: 'POST',
      path: `/v1/applications/${encodeURIComponent(id)}/notes`,
      body: { note },
    })
    return toApplication(response)
  }
}
