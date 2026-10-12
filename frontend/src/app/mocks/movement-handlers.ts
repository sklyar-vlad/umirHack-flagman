import { writtenOffMovements } from './write-off-fixtures'
import { delay } from 'msw'
import { http } from 'msw/http'
import { movementSearchError, readMovementSearch } from '@/entities/stock-movement'
import { compareHistoryInstants } from '@/shared/lib/history'
import { mockExtraOrder, mockMovements, mockStockLedger } from './stock-ledger-fixtures'
export { mockMovements } from './stock-ledger-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

export const movementHandlers = [http.get('/api/v1/stock-movements', async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (scenario === 'forbidden' || scenario === 'movements-forbidden' || scenario === 'role-order-operator') return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (scenario === 'movements-slow') await delay(2000)
  if (scenario === 'server-error' || scenario === 'movements-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
  if (scenario === 'network-error') return Response.error()
  if (scenario === 'movements-invalid') return Response.json({ items: [{ ...mockMovements[0], delta_quantity: '-1000.000' }], total: 1, page: 1, page_size: 50, meta: {} })
  const values = readMovementSearch(new URL(request.url).searchParams)
  if (movementSearchError(values)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте фильтры')
  const ledger = scenario === 'losses-order' ? [...mockStockLedger, mockExtraOrder] : mockStockLedger
  const items = (scenario === 'movements-empty' ? [] : [...ledger, ...writtenOffMovements()].sort((a, b) => compareHistoryInstants(b.occurred_at, a.occurred_at))).filter((item) =>
    (!values.product_id || item.product_id === values.product_id) && (!values.batch_id || item.batch_id === values.batch_id) &&
    (!values.order_id || item.order_id === values.order_id) && (!values.type || item.type === values.type) &&
    (!values.from || compareHistoryInstants(item.occurred_at, values.from) >= 0) && (!values.to || compareHistoryInstants(item.occurred_at, values.to) < 0))
  return Response.json({ items: items.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: items.length, page: values.page, page_size: values.page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
