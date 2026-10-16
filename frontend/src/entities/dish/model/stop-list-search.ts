export interface StopListSearch { search: string; page: number; page_size: number }
export const defaultStopListSearch: StopListSearch = { search: '', page: 1, page_size: 20 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readStopListSearch(params: URLSearchParams): StopListSearch {
  return { search: params.get('search') ?? '', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 20, 100) }
}
export function stopListSearchParams(values: StopListSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultStopListSearch) as (keyof StopListSearch)[]) if (values[key] !== defaultStopListSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeStopListSearch(values: StopListSearch, patch: Partial<StopListSearch>): StopListSearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
