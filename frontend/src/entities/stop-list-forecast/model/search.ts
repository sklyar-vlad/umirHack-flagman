import { z } from 'zod'

export const forecastSearchSchema = z.object({
  lookback_days: z.number().int().min(1).max(90), horizon_hours: z.number().int().min(1).max(720),
  page: z.number().int().positive().safe(), page_size: z.number().int().min(1).max(100),
})
export type ForecastSearch = z.infer<typeof forecastSearchSchema>
export const defaultForecastSearch: ForecastSearch = { lookback_days: 7, horizon_hours: 72, page: 1, page_size: 50 }
export function readForecastSearch(params: URLSearchParams): ForecastSearch {
  const read = (key: keyof ForecastSearch) => {
    const values = params.getAll(key)
    return !values.length ? defaultForecastSearch[key] : values.length === 1 && /^\d+$/.test(values[0]) ? Number(values[0]) : NaN
  }
  return { lookback_days: read('lookback_days'), horizon_hours: read('horizon_hours'), page: read('page'), page_size: read('page_size') }
}
export function forecastSearchError(values: ForecastSearch): string | null {
  return forecastSearchSchema.safeParse(values).success ? null : 'История: 1–90 дней; горизонт: 1–720 часов; страница — положительное целое; размер страницы: 1–100. Параметры не должны повторяться.'
}
export function forecastSearchParams(values: ForecastSearch): URLSearchParams {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultForecastSearch) as (keyof ForecastSearch)[]) if (values[key] !== defaultForecastSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeForecastSearch(values: ForecastSearch, patch: Partial<ForecastSearch>): ForecastSearch {
  return { ...values, ...patch, page: patch.page ?? 1 }
}
