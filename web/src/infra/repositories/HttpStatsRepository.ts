import type { Stats } from '@/core/domain/models'
import type { ReadOptions } from '@/core/ports/applications'
import type { StatsReader } from '@/core/ports/stats'
import type { Transport } from '@/core/ports/transport'
import { toStats } from './mapping'

export class HttpStatsRepository implements StatsReader {
  readonly #transport: Transport

  constructor(transport: Transport) {
    this.#transport = transport
  }

  async get(options?: ReadOptions): Promise<Stats> {
    const response = await this.#transport.send<unknown>({
      method: 'GET',
      path: '/v1/stats',
      ...(options?.signal ? { signal: options.signal } : {}),
    })
    return toStats(response)
  }
}
