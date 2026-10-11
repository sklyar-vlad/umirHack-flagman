import { readHistorySearch as readSupplySearch, historyPeriodError as supplyPeriodError, historySearchParams as supplySearchParams } from '@/shared/lib/history'
export { defaultHistorySearch as defaultSupplySearch, compareHistoryInstants as compareSupplyInstants, readHistorySearch as readSupplySearch, historyPeriodError as supplyPeriodError, historySearchParams as supplySearchParams, changeHistorySearch as changeSupplySearch, historyInstantToInput as supplyInstantToInput, historyFilterSchema as supplyFilterSchema } from '@/shared/lib/history'
export type { HistorySearch as SupplySearch } from '@/shared/lib/history'

/** Only the supply-list path and its non-secret filters may be used as a return link. */
export function safeSupplyListUrl(raw: string | null): string {
  if (!raw || !raw.startsWith('/supplies?') && raw !== '/supplies') return '/supplies'
  const url = new URL(raw, 'https://internal.invalid')
  const values = readSupplySearch(url.searchParams)
  if (supplyPeriodError(values)) { values.from = ''; values.to = '' }
  const search = supplySearchParams(values).toString()
  return `/supplies${search ? `?${search}` : ''}`
}
