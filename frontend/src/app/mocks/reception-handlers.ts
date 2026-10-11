import { catalogProductHandlers } from './catalog-handlers'
import { delay } from 'msw'
import { http } from 'msw/http'
import { z } from 'zod'
import { createSupplyRequestSchema } from '@/shared/api'
import { acceptReception, receptionProducts, savedReception } from './reception-fixtures'
import { createMockError, createHealthcheckResponse } from './responses'
import { getMockScenario } from './scenarios'

export const receptionHandlers = [
  ...catalogProductHandlers,
  http.post('/api/v1/supplies', async ({ request }) => {
    const scenario = getMockScenario()
    if (['network-error', 'unauthorized', 'forbidden', 'server-error', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
    if (scenario === 'reception-slow') await delay(2000)
    const key = request.headers.get('Idempotency-Key') ?? ''
    if (!z.uuid().safeParse(key).success) return createMockError(422, 'VALIDATION_ERROR', 'Нужен UUID Idempotency-Key')
    const parsed = createSupplyRequestSchema.safeParse(await request.json())
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Некорректные строки')
    const body = parsed.data, prior = savedReception(key)
    if (prior) {
      if (prior.body !== JSON.stringify(body)) return createMockError(409, 'IDEMPOTENCY_CONFLICT', 'Ключ использован с другим телом')
      return Response.json(prior.supply, { status: 201 })
    }
    if (scenario === 'reception-in-progress') return createMockError(409, 'REQUEST_IN_PROGRESS', 'Запрос ещё выполняется', {}, '1')
    if (scenario === 'reception-network') return Response.error()
    if (scenario === 'reception-conflict') return createMockError(409, 'CONFLICT', 'Номер уже занят')
    if (scenario === 'reception-validation') return createMockError(422, 'VALIDATION_ERROR', 'Проверьте строки', { fields: [{ path: `items[${body.items.length - 1}].expires_at`, code: 'expired', message: 'Срок этой партии уже истёк на сервере.' }] })
    for (const [index, line] of body.items.entries()) {
      if (!receptionProducts.some((item) => item.id === line.product_id) || Date.parse(line.expires_at) <= Date.now()) return createMockError(422, 'VALIDATION_ERROR', 'Строка отклонена', { fields: [{ path: `items[${index}].expires_at`, code: 'expired', message: 'Проверьте активность продукта и актуальный срок.' }] })
    }
    const supply = acceptReception(key, body)
    if (scenario === 'reception-lost-response') return Response.error()
    return Response.json(supply, { status: 201, headers: { 'Location': `/api/v1/supplies/${supply.id}`, 'X-Mock-Mode': 'true' } })
  }),
]
