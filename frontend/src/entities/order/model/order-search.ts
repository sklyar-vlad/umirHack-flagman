import { defaultHistorySearch, historyPeriodError, historySearchParams, readHistorySearch, type HistorySearch } from '@/shared/lib/history'
import { orderReasonSchema, type OrderReason } from './contracts'

export type OrderTab = 'orders' | 'attempts'
export interface AttemptSearch extends HistorySearch { reason_code: OrderReason | '' }
export interface OrderSearch { tab: OrderTab; orders: HistorySearch; attempts: AttemptSearch }
export const defaultOrderSearch: OrderSearch = { tab: 'orders', orders: { ...defaultHistorySearch }, attempts: { ...defaultHistorySearch, reason_code: '' } }
export function readOrderSearch(params: URLSearchParams): OrderSearch {
  const read = (tab: OrderTab) => {
    const inner = new URLSearchParams()
    for (const key of Object.keys(defaultHistorySearch)) {
      const value = params.get(`${tab}_${key}`)
      if (value !== null) inner.set(key, value)
    }
    return readHistorySearch(inner)
  }
  return { tab: params.get('tab') === 'attempts' ? 'attempts' : 'orders', orders: read('orders'), attempts: { ...read('attempts'), reason_code: (params.get('attempts_reason_code') ?? '') as AttemptSearch['reason_code'] } }
}
export function orderSearchError(values: HistorySearch | AttemptSearch): string | null {
  return historyPeriodError(values) || ('reason_code' in values && values.reason_code !== '' && !orderReasonSchema.safeParse(values.reason_code).success ? 'Некорректная причина отказа в адресе.' : null)
}
export function orderSearchParams(values: OrderSearch): URLSearchParams {
  const params = new URLSearchParams()
  if (values.tab === 'attempts') params.set('tab', 'attempts')
  for (const tab of ['orders', 'attempts'] as const) historySearchParams(values[tab]).forEach((value, key) => params.set(`${tab}_${key}`, value))
  if (values.attempts.reason_code) params.set('attempts_reason_code', values.attempts.reason_code)
  return params
}
export function changeOrderSearch(values: OrderSearch, patch: Partial<AttemptSearch>): OrderSearch {
  const tab = values.tab
  if (tab === 'orders') { patch = { ...patch }; delete patch.reason_code }
  return { ...values, [tab]: { ...values[tab], ...patch, page: patch.page ?? 1 } }
}
export function safeOrderListUrl(raw: string | null): string {
  if (!raw || raw !== '/orders' && !raw.startsWith('/orders?')) return '/orders'
  const values = readOrderSearch(new URL(raw, 'https://internal.invalid').searchParams)
  if (orderSearchError(values.orders)) values.orders = { ...defaultOrderSearch.orders }
  if (orderSearchError(values.attempts)) values.attempts = { ...defaultOrderSearch.attempts }
  const search = orderSearchParams(values).toString()
  return `/orders${search ? `?${search}` : ''}`
}
