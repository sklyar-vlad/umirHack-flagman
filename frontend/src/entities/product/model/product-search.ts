import { z } from 'zod'
export interface ProductSearch { search: string; category_id: string; is_active: 'all' | 'true' | 'false'; sort: 'name'; page: number; page_size: number }
export const defaultProductSearch: ProductSearch = { search: '', category_id: '', is_active: 'all', sort: 'name', page: 1, page_size: 50 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readProductSearch(params: URLSearchParams): ProductSearch {
  const category = params.get('category_id') ?? '', active = params.get('is_active')
  return { search: params.get('search') ?? '', category_id: z.uuid().safeParse(category).success ? category : '', is_active: active === 'true' || active === 'false' ? active : 'all', sort: 'name', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function productSearchParams(values: ProductSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultProductSearch) as (keyof ProductSearch)[]) if (values[key] !== defaultProductSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeProductSearch(values: ProductSearch, patch: Partial<ProductSearch>): ProductSearch { return { ...values, ...patch, page: patch.page ?? 1 } }

/** A list return destination contains published filters only. */
export function safeProductListUrl(value: string | null | undefined) {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return '/admin/products'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || url.pathname !== '/admin/products') return '/admin/products'
  const params = productSearchParams(readProductSearch(url.searchParams))
  return url.pathname + (params.size ? `?${params}` : '')
}
