import { z } from 'zod'
import { parseApiDateTime, restaurantDateTimeFromInput, restaurantDateTimeToInput } from '@/shared/lib/restaurant-time'
const historyInstantSchema = z.string().refine((value) => {
  try { parseApiDateTime(value); return true } catch { return false }
}, 'Укажите дату и время с часовым поясом.')

export interface HistorySearch { search: string; from: string; to: string; sort: '-created_at'; page: number; page_size: number }
export const defaultHistorySearch: HistorySearch = { search: '', from: '', to: '', sort: '-created_at', page: 1, page_size: 50 }
export function compareHistoryInstants(a: string, b: string): number {
  const nanos = (raw: string) => BigInt(parseApiDateTime(raw).getTime()) * 1_000_000n + BigInt((raw.match(/\.(\d+)/)?.[1] ?? '').padEnd(9, '0').slice(3))
  const aa = nanos(a), bb = nanos(b)
  return aa < bb ? -1 : aa > bb ? 1 : 0
}
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readHistorySearch(params: URLSearchParams): HistorySearch {
  return { search: params.get('search') ?? '', from: params.get('from') ?? '', to: params.get('to') ?? '', sort: '-created_at', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function historyPeriodError(values: Pick<HistorySearch, 'from' | 'to'>): string | null {
  if ([values.from, values.to].some((value) => value && !historyInstantSchema.safeParse(value).success)) return 'Период в адресе некорректен. Укажите дату и время с часовым поясом.'
  if (values.from && values.to && compareHistoryInstants(values.from, values.to) > 0) return 'Начало периода должно быть не позже конца.'
  return null
}
export function historySearchParams(values: HistorySearch): URLSearchParams {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultHistorySearch) as (keyof HistorySearch)[]) {
    if (values[key] !== defaultHistorySearch[key]) params.set(key, String(values[key]))
  }
  return params
}
export function changeHistorySearch(values: HistorySearch, patch: Partial<HistorySearch>): HistorySearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
export function historyInstantToInput(raw: string, timezone: string): string {
  try {
    return (restaurantDateTimeToInput(raw || null, timezone) ?? '').replace(/(\.\d{3})\d+$/, '$1')
      .replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '').replace(/:00$/, '')
  } catch { return '' }
}
export function historyFilterSchema(timezone: string, original?: Pick<HistorySearch, 'from' | 'to'>) {
  const local = (prior = '') => z.string().transform((value, ctx) => {
    if (prior && historyInstantSchema.safeParse(prior).success && value === historyInstantToInput(prior, timezone)) return prior
    try { return restaurantDateTimeFromInput(value, timezone) ?? '' }
    catch (error) { ctx.addIssue({ code: 'custom', message: error instanceof Error ? error.message : 'Некорректная дата.' }); return z.NEVER }
  })
  return z.object({ search: z.string(), from: local(original?.from), to: local(original?.to) }).refine((values) => !historyPeriodError(values), { path: ['to'], message: 'Начало периода должно быть не позже конца.' })
}
export function convertHistoryTimezone<T extends { search: string; from: string; to: string }>(values: T, original: Pick<HistorySearch, 'from' | 'to'>, from: string, to: string): T {
  const instants = historyFilterSchema(from, original).parse(values)
  const projected = { ...values, from: historyInstantToInput(instants.from, to), to: historyInstantToInput(instants.to, to) }
  historyFilterSchema(to).parse(projected)
  return projected
}
