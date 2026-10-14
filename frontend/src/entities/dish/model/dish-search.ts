export interface DishSearch { search: string; availability: 'all' | 'available' | 'unavailable'; is_active: 'all' | 'true' | 'false'; sort: 'name'; page: number; page_size: number }
export const defaultDishSearch: DishSearch = { search: '', availability: 'all', is_active: 'all', sort: 'name', page: 1, page_size: 50 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readDishSearch(params: URLSearchParams): DishSearch {
  const availability = params.get('availability'), active = params.get('is_active')
  return { search: params.get('search') ?? '', availability: availability === 'available' || availability === 'unavailable' ? availability : 'all', is_active: active === 'true' || active === 'false' ? active : 'all', sort: 'name', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function dishSearchParams(values: DishSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultDishSearch) as (keyof DishSearch)[]) if (values[key] !== defaultDishSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeDishSearch(values: DishSearch, patch: Partial<DishSearch>): DishSearch { return { ...values, ...patch, page: patch.page ?? 1 } }

/** A list return destination contains published filters only. */
export function safeDishListUrl(value: string | null | undefined) {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return '/admin/dishes'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || url.pathname !== '/admin/dishes') return '/admin/dishes'
  const params = dishSearchParams(readDishSearch(url.searchParams))
  return url.pathname + (params.size ? `?${params}` : '')
}

/** Viewing recipes returns only to the public catalogue, with published filters. */
export function safeRecipeListUrl(value: string | null | undefined) {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return '/dishes'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || url.pathname !== '/dishes') return '/dishes'
  const params = dishSearchParams(readDishSearch(url.searchParams))
  return url.pathname + (params.size ? `?${params}` : '')
}
