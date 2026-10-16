export interface HotOfferSearch { search: string; page: number; page_size: number }
export const defaultHotOfferSearch: HotOfferSearch = { search: '', page: 1, page_size: 50 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readHotOfferSearch(params: URLSearchParams): HotOfferSearch {
  return { search: params.get('search') ?? '', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function hotOfferSearchParams(values: HotOfferSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultHotOfferSearch) as (keyof HotOfferSearch)[]) if (values[key] !== defaultHotOfferSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeHotOfferSearch(values: HotOfferSearch, patch: Partial<HotOfferSearch>): HotOfferSearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
