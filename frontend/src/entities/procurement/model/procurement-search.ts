import { z } from 'zod'
export interface ProcurementSearch { search: string; category_id: string; page: number; page_size: number }
export const defaultProcurementSearch: ProcurementSearch = { search: '', category_id: '', page: 1, page_size: 20 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readProcurementSearch(params: URLSearchParams): ProcurementSearch {
  const category = params.get('category_id') ?? ''
  return { search: params.get('search') ?? '', category_id: z.uuid().safeParse(category).success ? category : '', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 20, 100) }
}
export function procurementSearchParams(values: ProcurementSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultProcurementSearch) as (keyof ProcurementSearch)[]) if (values[key] !== defaultProcurementSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeProcurementSearch(values: ProcurementSearch, patch: Partial<ProcurementSearch>): ProcurementSearch { return { ...values, ...patch, page: patch.page ?? 1 } }
