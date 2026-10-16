import { writtenOffMovements } from './write-off-fixtures'
import { delay } from 'msw'
import { http } from 'msw/http'
import { lossQuantity, lossSearchError, readLossSearch, type LossRow } from '@/entities/write-off-report'
import { compareHistoryInstants } from '@/shared/lib/history'
import { mockExtraOrder, mockStockLedger as lossOperations } from './stock-ledger-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

const decimal = (n: bigint) => `${n / 1000n}.${String(n % 1000n).padStart(3, '0')}`
export const lossHandlers = [http.get('/api/v1/analytics/write-offs', async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (['forbidden', 'losses-forbidden', 'role-storekeeper', 'role-order-operator'].includes(scenario)) return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (scenario === 'losses-slow') await delay(2000)
  if (scenario === 'losses-error' || scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
  if (scenario === 'network-error') return Response.error()
  if (scenario === 'losses-invalid') return Response.json({ items: [{}], total: 1, page: 1, page_size: 50, meta: {} })
  const values = readLossSearch(new URL(request.url).searchParams)
  if (lossSearchError(values)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте фильтры')
  const groups = new Map<string, LossRow>()
  const operations = scenario === 'losses-order' ? [...lossOperations, mockExtraOrder] : [...lossOperations, ...writtenOffMovements()]
  for (const op of scenario === 'losses-empty' ? [] : operations) {
    if (!['expiration', 'manual_writeoff'].includes(op.type) || values.reason !== 'all' && op.type !== values.reason || values.product_id && op.product_id !== values.product_id || values.from && compareHistoryInstants(op.occurred_at, values.from) < 0 || values.to && compareHistoryInstants(op.occurred_at, values.to) >= 0) continue
    const key = `${op.product_id}:${op.unit}`, quantity = lossQuantity(op.delta_quantity.slice(1))
    const row = groups.get(key) ?? { product_id: op.product_id, product_name: op.product_name, unit: op.unit, expired_written_off_quantity: '0.000', manual_written_off_quantity: '0.000', total_written_off_quantity: '0.000', movements_count: 0 }
    const field = op.type === 'expiration' ? 'expired_written_off_quantity' : 'manual_written_off_quantity'
    row[field] = decimal(lossQuantity(row[field]) + quantity)
    row.total_written_off_quantity = decimal(lossQuantity(row.total_written_off_quantity) + quantity)
    row.movements_count++; groups.set(key, row)
  }
  const rows = [...groups.values()].sort((a, b) => a.product_name.localeCompare(b.product_name, 'ru') || a.product_id.localeCompare(b.product_id) || a.unit.localeCompare(b.unit))
  return Response.json({ items: rows.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: rows.length, page: values.page, page_size: values.page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
