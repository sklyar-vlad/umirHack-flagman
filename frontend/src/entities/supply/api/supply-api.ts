import { z } from 'zod'
import { baseApi, createSupplyRequestSchema, type CreateSupplyRequest, type ApiError } from '@/shared/api'
import { parseQuantity } from '@/shared/lib/quantity'
import { supplyDetailSchema, supplyListSchema, type SupplyDetail, type SupplyList } from '../model/contracts'
import { compareSupplyInstants, supplyPeriodError, type SupplySearch } from '../model/supply-search'

function invalidResponse(): ApiError {
  return { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сервера не соответствует опубликованному контракту поставок.', details: {} }
}
export const supplyApi = baseApi.enhanceEndpoints({ addTagTypes: ['Supplies', 'Inventory', 'Batches', 'Dashboard', 'StopListForecast', 'Dishes', 'StopList', 'Recommendations', 'Procurement'] }).injectEndpoints({
  endpoints: (build) => ({
    receiveSupply: build.mutation<SupplyDetail, { key: string; body: CreateSupplyRequest }>({
      invalidatesTags: (result) => result ? ['Supplies', 'Inventory', 'Batches', 'Dashboard', 'StopListForecast', 'Dishes', 'StopList', 'Recommendations', 'Procurement'] : [],
      queryFn: async ({ key, body }, _api, _options, query) => {
        if (!z.uuid().safeParse(key).success || !createSupplyRequestSchema.safeParse(body).success) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Некорректная попытка приёмки.', details: {} } }
        const result = await query({ url: '/supplies', method: 'POST', body, headers: { 'Idempotency-Key': key } })
        if (result.error) return { error: result.error }
        const parsed = supplyDetailSchema.safeParse(result.data)
        if (result.meta?.response?.status !== 201 || !parsed.success || parsed.data.number !== body.number || compareSupplyInstants(parsed.data.received_at, body.received_at) !== 0 || parsed.data.items.length !== body.items.length || parsed.data.items.some((line, index) => line.product.id !== body.items[index].product_id || line.batch.line_number !== index + 1 || parseQuantity(line.batch.initial_quantity) !== parseQuantity(body.items[index].quantity) || compareSupplyInstants(line.batch.expires_at, body.items[index].expires_at) !== 0)) return { error: invalidResponse() }
        return { data: parsed.data }
      },
    }),
    supply: build.query<SupplyDetail, string>({
      providesTags: ['Supplies'],
      queryFn: async (id, _api, _options, query) => {
        if (!z.uuid().safeParse(id).success) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Некорректный идентификатор поставки.', details: {} } }
        const result = await query({ url: `/supplies/${encodeURIComponent(id)}` })
        if (result.error) return { error: result.error }
        const parsed = supplyDetailSchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalidResponse() }
      },
    }),
    supplies: build.query<SupplyList, SupplySearch>({
      providesTags: ['Supplies'],
      queryFn: async (values, _api, _options, query) => {
        if (supplyPeriodError(values)) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Некорректный период.', details: {} } }
        const { search, from, to, ...rest } = values
        const result = await query({ url: '/supplies', params: { ...rest, ...(search && { search }), ...(from && { from }), ...(to && { to }) } })
        if (result.error) return { error: result.error }
        const parsed = supplyListSchema.safeParse(result.data)
        if (!parsed.success || parsed.data.page !== values.page || parsed.data.page_size !== values.page_size || parsed.data.items.length > values.page_size || parsed.data.items.length > parsed.data.total || new Set(parsed.data.items.map((item) => item.id)).size !== parsed.data.items.length) return { error: invalidResponse() }
        return { data: parsed.data }
      },
    }),
  }),
})
export const { useSuppliesQuery, useSupplyQuery, useReceiveSupplyMutation } = supplyApi
/** Dispatch after a successful reception, including when its response is replayed. */
export const invalidateSupplies = () => supplyApi.util.invalidateTags(['Supplies'])
