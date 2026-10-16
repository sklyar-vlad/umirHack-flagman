import { historyPeriodError, readHistorySearch } from '@/shared/lib/history'

export interface ShortageSearch { from: string; to: string; page: number; page_size: number }
export const defaultShortageSearch: ShortageSearch = { from: '', to: '', page: 1, page_size: 50 }
export function readShortageSearch(params: URLSearchParams): ShortageSearch {
  const { from, to, page, page_size } = readHistorySearch(params)
  return { from, to, page, page_size }
}
export const shortageSearchError = historyPeriodError
export function shortageSearchParams(values: ShortageSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultShortageSearch) as (keyof ShortageSearch)[]) {
    if (values[key] !== defaultShortageSearch[key]) params.set(key, String(values[key]))
  }
  return params
}
export function changeShortageSearch(values: ShortageSearch, patch: Partial<ShortageSearch>): ShortageSearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
