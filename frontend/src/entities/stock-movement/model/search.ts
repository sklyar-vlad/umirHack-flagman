import { z } from 'zod'
import { historyPeriodError, readHistorySearch } from '@/shared/lib/history'
import { movementTypeSchema } from './contracts'

export interface MovementSearch { product_id: string; batch_id: string; order_id: string; type: string; from: string; to: string; sort: '-occurred_at'; page: number; page_size: number }
export const defaultMovementSearch: MovementSearch = { product_id: '', batch_id: '', order_id: '', type: '', from: '', to: '', sort: '-occurred_at', page: 1, page_size: 50 }
export function readMovementSearch(params: URLSearchParams): MovementSearch {
  const { from, to, page, page_size } = readHistorySearch(params)
  return { ...defaultMovementSearch, from, to, page, page_size, product_id: (params.get('product_id') ?? '').toLowerCase(), batch_id: (params.get('batch_id') ?? '').toLowerCase(), order_id: (params.get('order_id') ?? '').toLowerCase(), type: params.get('type') ?? '' }
}
export function movementSearchError(values: MovementSearch): string | null {
  return historyPeriodError(values) || (['product_id', 'batch_id', 'order_id'] as const).some((key) => values[key] && !z.uuid().safeParse(values[key]).success) && 'Проверьте ID продукта, партии и заказа: ожидаются UUID.' || values.type && !movementTypeSchema.safeParse(values.type).success && 'Некорректный тип движения в адресе.' || null
}
export function movementSearchParams(values: MovementSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultMovementSearch) as (keyof MovementSearch)[]) if (values[key] !== defaultMovementSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeMovementSearch(values: MovementSearch, patch: Partial<MovementSearch>): MovementSearch { return { ...values, ...patch, page: patch.page ?? 1 } }
