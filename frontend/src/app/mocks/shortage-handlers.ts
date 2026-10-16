import { delay } from 'msw'
import { http } from 'msw/http'
import { readShortageSearch, shortageSearchError } from '@/entities/ingredient-shortage'
import { shortageAttempts, shortageRows } from './shortage-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

export const shortageHandlers = [http.get('/api/v1/analytics/ingredient-shortages', async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (['forbidden', 'shortages-forbidden', 'role-storekeeper', 'role-order-operator'].includes(scenario)) return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (scenario === 'shortages-slow' || scenario === 'slow') await delay(2000)
  if (scenario === 'shortages-error' || scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
  if (scenario === 'network-error') return Response.error()
  if (scenario === 'shortages-invalid') return Response.json({ items: [{}], total: 1, page: 1, page_size: 50, meta: {} })
  const values = readShortageSearch(new URL(request.url).searchParams)
  if (shortageSearchError(values)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте период')
  const rows = scenario === 'shortages-empty' ? [] : shortageRows(values, shortageAttempts(scenario === 'shortages-demo'))
  return Response.json({ items: rows.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: rows.length, page: values.page, page_size: values.page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
