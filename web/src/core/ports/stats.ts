import type { Stats } from '@/core/domain/models'
import type { ReadOptions } from './applications'

export interface StatsReader {
  get(options?: ReadOptions): Promise<Stats>
}
