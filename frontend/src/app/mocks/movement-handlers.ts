import { delay } from 'msw'
import { http } from 'msw/http'
import { movementSearchError, readMovementSearch, type StockMovement } from '@/entities/stock-movement'
import { compareHistoryInstants } from '@/shared/lib/history'
import { inventoryBatches, mockId } from './inventory-fixtures'
import { orderDetailFixture } from './order-detail-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

// Synthetic historical snapshots deliberately differ from today's inventory names and balances.
export const mockMovements: StockMovement[] = Array.from({ length: 65 }, (_, index) => {
  const type = (['receipt', 'order_consumption', 'manual_writeoff', 'expiration'] as const)[index % 4]
  const batch = inventoryBatches[type === 'receipt' ? index / 4 : index % 3]
  return { id: mockId(2000 + index), product_id: batch.product_id, batch_id: batch.id,
    order_id: type === 'order_consumption' ? orderDetailFixture.id : null, order_item_id: type === 'order_consumption' ? orderDetailFixture.items[0].id : null,
    type, delta_quantity: type === 'receipt' ? '1000.000' : type === 'order_consumption' ? '-12.125' : '-100.000',
    remaining_quantity_after: type === 'receipt' ? '1000.000' : type === 'order_consumption' ? '987.875' : '0.000',
    product_name: 'Молоко · название при операции', unit: 'ml', actor: type === 'expiration' ? null : { id: mockId(800), name: 'Анна Смирнова' },
    occurred_at: new Date(Date.UTC(2026, 9, 10, 9) - index * 60000).toISOString(), note: type === 'expiration' ? 'Истечение срока годности' : type === 'manual_writeoff' ? 'Повреждена упаковка' : null }
})

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
  const items = (scenario === 'movements-empty' ? [] : mockMovements).filter((item) =>
    (!values.product_id || item.product_id === values.product_id) && (!values.batch_id || item.batch_id === values.batch_id) &&
    (!values.order_id || item.order_id === values.order_id) && (!values.type || item.type === values.type) &&
    (!values.from || compareHistoryInstants(item.occurred_at, values.from) >= 0) && (!values.to || compareHistoryInstants(item.occurred_at, values.to) < 0))
  return Response.json({ items: items.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: items.length, page: values.page, page_size: values.page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
