import { z } from 'zod'
import { compareHistoryInstants } from '@/shared/lib/history'
import { baseApi, batchSchema, type ApiError } from '@/shared/api'
import { movementsSchema, type StockMovement } from '../model/contracts'
import type { MovementSearch } from '../model/search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ истории движений.', details: {} }
const batches = z.object({ items: z.array(batchSchema), total: z.number().int().nonnegative().safe(), page: z.number().int().positive().safe(), page_size: z.literal(100), meta: z.object({}) })
export const movementApi = baseApi.enhanceEndpoints({ addTagTypes: ['StockMovements', 'Batches'] }).injectEndpoints({ endpoints: (build) => ({
  stockMovements: build.query<ReturnType<typeof movementsSchema.parse>, MovementSearch>({
    providesTags: ['StockMovements'],
    queryFn: async (values, _api, _options, query) => {
      const result = await query({ url: '/stock-movements', params: Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '')) })
      if (result.error) return { error: result.error }
      const parsed = movementsSchema.safeParse(result.data)
      if (!parsed.success) return { error: invalid }
      const data = parsed.data
      if (data.page !== values.page || data.page_size !== values.page_size || data.items.length > data.page_size || data.items.length > data.total || new Set(data.items.map((item) => item.id)).size !== data.items.length) return { error: invalid }
      if (data.items.some((item) => values.product_id && item.product_id !== values.product_id || values.batch_id && item.batch_id !== values.batch_id || values.order_id && item.order_id !== values.order_id || values.type && item.type !== values.type || values.from && compareHistoryInstants(item.occurred_at, values.from) < 0 || values.to && compareHistoryInstants(item.occurred_at, values.to) >= 0)) return { error: invalid }
      if (data.items.some((item, index) => index > 0 && compareHistoryInstants(data.items[index - 1].occurred_at, item.occurred_at) < 0)) return { error: invalid }
      return { data }
    },
  }),
  movementSupply: build.query<z.infer<typeof batchSchema> | null, Pick<StockMovement, 'product_id' | 'batch_id'>>({
    providesTags: ['Batches'],
    // StockMovement has no supply_id. Resolve only the selected batch through the published batches API.
    queryFn: async ({ product_id, batch_id }, _api, _options, query) => {
      let total: number | undefined
      const seen = new Set<string>()
      for (let page = 1; page <= 100; page++) {
        const result = await query({ url: `/products/${product_id}/batches`, params: { status: 'all', sort: 'expires_at', page, page_size: 100 } })
        if (result.error) return { error: result.error }
        const parsed = batches.safeParse(result.data)
        if (!parsed.success) return { error: invalid }
        const data = parsed.data
        if (data.page !== page || data.items.length > 100 || data.total < seen.size + data.items.length || total !== undefined && total !== data.total || data.items.some((item) => item.product_id !== product_id || seen.has(item.id)) || new Set(data.items.map((item) => item.id)).size !== data.items.length) return { error: invalid }
        total = data.total
        data.items.forEach((item) => seen.add(item.id))
        const batch = data.items.find((item) => item.id === batch_id)
        if (batch) return { data: batch }
        if (seen.size === total) return { data: null }
        if (!data.items.length) return { error: invalid }
      }
      return { error: { ...invalid, message: 'Слишком много партий для поиска поставки. Откройте партии на складе.' } }
    },
  }),
}) })
export const { useStockMovementsQuery, useLazyMovementSupplyQuery } = movementApi
