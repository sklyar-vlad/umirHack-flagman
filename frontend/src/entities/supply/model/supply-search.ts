import { z } from 'zod'
import { parseApiDateTime, restaurantDateTimeFromInput, restaurantDateTimeToInput } from '@/shared/lib/restaurant-time'
import { supplyInstantSchema } from './contracts'

export interface SupplySearch { search: string; from: string; to: string; sort: '-created_at'; page: number; page_size: number }
export const defaultSupplySearch: SupplySearch = { search: '', from: '', to: '', sort: '-created_at', page: 1, page_size: 50 }
/** Preserve the published fractional seconds when comparing period bounds. */
export function compareSupplyInstants(a: string, b: string): number {
  const nanos = (raw: string) => BigInt(parseApiDateTime(raw).getTime()) * 1_000_000n + BigInt((raw.match(/\.(\d+)/)?.[1] ?? '').padEnd(9, '0').slice(3))
  const aa = nanos(a), bb = nanos(b)
  return aa < bb ? -1 : aa > bb ? 1 : 0
}
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readSupplySearch(params: URLSearchParams): SupplySearch {
  return { search: params.get('search') ?? '', from: params.get('from') ?? '', to: params.get('to') ?? '', sort: '-created_at', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function supplyPeriodError(values: Pick<SupplySearch, 'from' | 'to'>): string | null {
  if ([values.from, values.to].some((value) => value && !supplyInstantSchema.safeParse(value).success)) return 'Период в адресе некорректен. Укажите дату и время с часовым поясом.'
  if (values.from && values.to && compareSupplyInstants(values.from, values.to) > 0) return 'Начало периода должно быть не позже конца.'
  return null
}
export function supplySearchParams(values: SupplySearch): URLSearchParams {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultSupplySearch) as (keyof SupplySearch)[]) {
    if (values[key] !== defaultSupplySearch[key]) params.set(key, String(values[key]))
  }
  return params
}
export function changeSupplySearch(values: SupplySearch, patch: Partial<SupplySearch>): SupplySearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
/** Only the supply-list path and its non-secret filters may be used as a return link. */
export function safeSupplyListUrl(raw: string | null): string {
  if (!raw || !raw.startsWith('/supplies?') && raw !== '/supplies') return '/supplies'
  const url = new URL(raw, 'https://internal.invalid')
  const values = readSupplySearch(url.searchParams)
  if (supplyPeriodError(values)) { values.from = ''; values.to = '' }
  const search = supplySearchParams(values).toString()
  return `/supplies${search ? `?${search}` : ''}`
}
/** Native datetime-local supports milliseconds; keep finer URL precision until the field changes. */
export function supplyInstantToInput(raw: string, timezone: string): string {
  try {
    return (restaurantDateTimeToInput(raw || null, timezone) ?? '').replace(/(\.\d{3})\d+$/, '$1')
      .replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '').replace(/:00$/, '')
  } catch { return '' }
}
export function supplyFilterSchema(timezone: string, original?: Pick<SupplySearch, 'from' | 'to'>) {
  const local = (prior = '') => z.string().transform((value, ctx) => {
    if (prior && supplyInstantSchema.safeParse(prior).success && value === supplyInstantToInput(prior, timezone)) return prior
    try { return restaurantDateTimeFromInput(value, timezone) ?? '' }
    catch (error) { ctx.addIssue({ code: 'custom', message: error instanceof Error ? error.message : 'Некорректная дата.' }); return z.NEVER }
  })
  return z.object({ search: z.string(), from: local(original?.from), to: local(original?.to) }).refine((values) => !supplyPeriodError(values), { path: ['to'], message: 'Начало периода должно быть не позже конца.' })
}
