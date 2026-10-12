import { z } from 'zod'
import { historyPeriodError, readHistorySearch } from '@/shared/lib/history'

export const lossReasonSchema = z.enum(['all', 'expiration', 'manual_writeoff'])
export interface LossSearch { product_id: string; reason: string; from: string; to: string; page: number; page_size: number }
export const defaultLossSearch: LossSearch = { product_id: '', reason: 'all', from: '', to: '', page: 1, page_size: 50 }
export function readLossSearch(params: URLSearchParams): LossSearch {
  const { from, to, page, page_size } = readHistorySearch(params)
  return { from, to, page, page_size, product_id: (params.get('product_id') ?? '').toLowerCase(), reason: params.get('reason') ?? 'all' }
}
export function lossSearchError(values: LossSearch): string | null {
  if (historyPeriodError(values)) return historyPeriodError(values)
  if (values.product_id && !z.uuid().safeParse(values.product_id).success) return 'Проверьте ID продукта: ожидается UUID.'
  if (!lossReasonSchema.safeParse(values.reason).success) return 'Некорректная причина списания в адресе.'
  return null
}
export function lossSearchParams(values: LossSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultLossSearch) as (keyof LossSearch)[]) if (values[key] !== defaultLossSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeLossSearch(values: LossSearch, patch: Partial<LossSearch>): LossSearch { return { ...values, ...patch, page: patch.page ?? 1 } }
export function lossMovementsUrl(row: Pick<import('./contracts').LossRow, 'product_id'>, values: Pick<LossSearch, 'from' | 'to'>, type: 'expiration' | 'manual_writeoff') {
  const params = new URLSearchParams({ product_id: row.product_id, type })
  if (values.from) params.set('from', values.from)
  if (values.to) params.set('to', values.to)
  return `/stock-movements?${params}`
}
