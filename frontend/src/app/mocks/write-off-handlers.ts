import { delay } from 'msw'
import { http } from 'msw/http'
import { writeOffRequestSchema } from '@/shared/api/write-off-work'
import { parseQuantity, serializeQuantity } from '@/shared/lib/quantity'
import type { StockMovement } from '@/entities/stock-movement'
import { z } from 'zod'
import { catalogInventoryItems } from './catalog-fixtures'
import { currentMockBatches } from './inventory-handlers'
import { mockId } from './inventory-fixtures'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'
import { saveMockWriteOff, savedMockWriteOff } from './write-off-fixtures'

export const writeOffHandlers = [http.post('/api/v1/write-offs', async ({ request }) => {
  const scenario = getMockScenario(), key = request.headers.get('Idempotency-Key') ?? ''
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (['forbidden', 'writeoff-forbidden', 'role-order-operator'].includes(scenario)) return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (request.headers.get('X-CSRF-Token') !== 'mock-csrf-for-local-tests') return createMockError(403, 'CSRF_INVALID', 'Неверный CSRF')
  if (!z.uuid().safeParse(key).success) return createMockError(422, 'VALIDATION_ERROR', 'Нужен ключ')
  const parsed = writeOffRequestSchema.safeParse(await request.json())
  if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте поля')
  const body = parsed.data, bodyText = JSON.stringify(body), prior = savedMockWriteOff(key)
  if (prior) return prior.body === bodyText ? Response.json(prior.payload, { status: prior.status, headers: { 'X-Mock-Mode': 'true' } }) : createMockError(409, 'IDEMPOTENCY_CONFLICT', 'Другое тело')
  if (scenario === 'writeoff-slow') await delay(2000)
  if (scenario === 'writeoff-network') return Response.error()
  if (scenario === 'writeoff-in-progress') return createMockError(409, 'REQUEST_IN_PROGRESS', 'Запрос выполняется')
  if (scenario === 'writeoff-validation') return createMockError(422, 'VALIDATION_ERROR', 'Проверьте поля', { fields: [{ path: 'reason', code: 'invalid', message: 'Причина отклонена' }, { path: 'internal_field', code: 'secret', message: 'Не показывать' }] })
  const reject = async (status: number, code: string, message: string) => {
    const response = createMockError(status, code, message)
    saveMockWriteOff({ key, body: bodyText, status, payload: await response.clone().json() })
    return response
  }
  if (scenario === 'writeoff-not-found') return reject(404, 'NOT_FOUND', 'Партия не найдена')
  let batch = currentMockBatches().find((item) => item.id === body.batch_id)
  if (!batch) return reject(404, 'NOT_FOUND', 'Партия не найдена')
  const product = catalogInventoryItems().find((item) => item.product.id === batch!.product_id)!.product
  if (batch.unit === 'pcs' && parseQuantity(body.quantity)! % 1000n !== 0n) return createMockError(422, 'VALIDATION_ERROR', 'Только целые штуки')
  if (scenario === 'writeoff-stock-changed' && batch.status !== 'expired' && !savedMockWriteOff(`external:${batch.id}`)) {
    const consumed = parseQuantity(batch.remaining_quantity)! / 2n
    const movement: StockMovement = { id: crypto.randomUUID(), batch_id: batch.id, product_id: batch.product_id, order_id: mockId(9001), order_item_id: mockId(9002), type: 'order_consumption', delta_quantity: `-${serializeQuantity(consumed)}`, remaining_quantity_after: serializeQuantity(parseQuantity(batch.remaining_quantity)! - consumed)!, product_name: product.name, unit: batch.unit, actor: { id: mockId(900), name: 'Тестовый оператор' }, occurred_at: new Date().toISOString(), note: null }
    saveMockWriteOff({ key: `external:${batch.id}`, body: '', status: 201, payload: movement, movement, expired: false })
    batch = currentMockBatches().find((item) => item.id === body.batch_id)!
  }
  if (parseQuantity(body.quantity)! > parseQuantity(batch.remaining_quantity)!) return reject(409, 'INSUFFICIENT_BATCH_STOCK', 'Остаток изменился')
  const movement: StockMovement = { id: crypto.randomUUID(), batch_id: batch.id, product_id: batch.product_id, order_id: null, order_item_id: null, type: 'manual_writeoff', delta_quantity: `-${body.quantity}`, remaining_quantity_after: serializeQuantity(parseQuantity(batch.remaining_quantity)! - parseQuantity(body.quantity)!)!, product_name: product.name, unit: batch.unit, actor: { id: mockId(900), name: 'Тестовый сотрудник' }, occurred_at: new Date().toISOString(), note: body.reason }
  saveMockWriteOff({ key, body: bodyText, status: 201, payload: movement, movement, expired: batch.status === 'expired' })
  if (scenario === 'writeoff-lost-response') return Response.error()
  if (scenario === 'writeoff-invalid') return Response.json({ ...movement, batch_id: mockId(9999) }, { status: 201 })
  return Response.json(movement, { status: 201, headers: { 'X-Mock-Mode': 'true' } })
})]
