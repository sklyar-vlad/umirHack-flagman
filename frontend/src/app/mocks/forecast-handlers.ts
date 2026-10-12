import { delay } from 'msw'
import { http } from 'msw/http'
import { readForecastSearch, forecastSearchError } from '@/entities/stop-list-forecast'
import { forecastRows, forecastServerTime } from './forecast-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

export const forecastHandlers = [http.get('/api/v1/analytics/stop-list-forecast', async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (['forbidden', 'forecast-forbidden', 'role-storekeeper', 'role-order-operator'].includes(scenario)) return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (scenario === 'forecast-slow' || scenario === 'slow') await delay(2000)
  if (scenario === 'forecast-error' || scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
  if (scenario === 'network-error') return Response.error()
  if (scenario === 'forecast-invalid') return Response.json({ items: [{}], total: 1, page: 1, page_size: 50, meta: {} })
  const values = readForecastSearch(new URL(request.url).searchParams)
  if (forecastSearchError(values)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте настройки')
  const rows = scenario === 'forecast-empty' ? [] : forecastRows(scenario === 'forecast-demo')
  return Response.json({ items: rows.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: rows.length, page: values.page, page_size: values.page_size, meta: { server_time: forecastServerTime, lookback_days: values.lookback_days, horizon_hours: values.horizon_hours } }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
