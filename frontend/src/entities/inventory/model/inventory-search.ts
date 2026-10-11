import { z } from 'zod'
import { inventoryFilters, type InventoryFilter } from './contracts'

export interface InventorySearch {
  search: string; category_id: string; filter: InventoryFilter;
  sort: 'name' | 'nearest_expires_at'; page: number; page_size: number
}
export const defaultInventorySearch: InventorySearch = { search: '', category_id: '', filter: 'all', sort: 'name', page: 1, page_size: 50 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
/** Browser URLs are untrusted; only contract parameters reach API requests. */
export function readInventorySearch(params: URLSearchParams): InventorySearch {
  const category = params.get('category_id') ?? ''
  const filter = params.get('filter')
  return {
    search: params.get('search') ?? '', category_id: z.uuid().safeParse(category).success ? category : '',
    filter: inventoryFilters.find((value) => value === filter) ?? 'all',
    sort: params.get('sort') === 'nearest_expires_at' ? 'nearest_expires_at' : 'name',
    page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100),
  }
}
export function inventorySearchParams(values: InventorySearch): URLSearchParams {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultInventorySearch) as (keyof InventorySearch)[]) {
    if (values[key] !== defaultInventorySearch[key]) params.set(key, String(values[key]))
  }
  return params
}
export function changeInventorySearch(values: InventorySearch, patch: Partial<InventorySearch>): InventorySearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
