import { z } from 'zod'
import { baseApi, type ApiError } from '@/shared/api'
import { writeOffRequestSchema, type WriteOffRequest } from '@/shared/api/write-off-work'
import { parseMovementQuantity, parseQuantity, type Unit } from '@/shared/lib/quantity'
import { movementSchema, type StockMovement } from '../model/contracts'

export const writeOffTags = ['Supplies', 'Inventory', 'Batches', 'Dashboard', 'StopListForecast', 'Dishes', 'StopList', 'Recommendations', 'HotOffers', 'Procurement', 'StockMovements', 'ProductFlow', 'WriteOffReport'] as const
export const writeOffApi = baseApi.enhanceEndpoints({ addTagTypes: [...writeOffTags] }).injectEndpoints({
  endpoints: (build) => ({
    writeOff: build.mutation<StockMovement, { key: string; body: WriteOffRequest; productId: string; unit: Unit }>({
      invalidatesTags: (result) => result ? [...writeOffTags] : [],
      queryFn: async ({ key, body, productId, unit }, _api, _options, query) => {
        if (!z.uuid().safeParse(key).success || !z.uuid().safeParse(productId).success || !writeOffRequestSchema.safeParse(body).success || unit === 'pcs' && parseQuantity(body.quantity)! % 1000n !== 0n) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Некорректная попытка списания.', details: {} } }
        const result = await query({ url: '/write-offs', method: 'POST', body, headers: { 'Idempotency-Key': key } })
        if (result.error) return { error: result.error }
        const parsed = movementSchema.safeParse(result.data)
        if (result.meta?.response?.status !== 201 || !parsed.success || parsed.data.type !== 'manual_writeoff' || parsed.data.batch_id.toLowerCase() !== body.batch_id.toLowerCase() || parsed.data.product_id.toLowerCase() !== productId.toLowerCase() || parsed.data.unit !== unit || !parsed.data.actor || parsed.data.note !== body.reason || parseMovementQuantity(parsed.data.delta_quantity) !== -parseQuantity(body.quantity)!) {
          const error: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ не подтверждает исходное списание. Сохранён точный запрос для проверки.', details: {} }
          return { error }
        }
        return { data: parsed.data }
      },
    }),
  }),
})
export const { useWriteOffMutation } = writeOffApi
