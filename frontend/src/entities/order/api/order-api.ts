import { z } from 'zod'
import { orderRequestSchema, type OrderRequest } from '@/shared/api/order-work'
import { baseApi, type ApiError } from '@/shared/api'
import type { HistorySearch } from '@/shared/lib/history'
import { orderListSchema, orderAttemptListSchema, type OrderList, type OrderAttemptList, orderCheckSchema, orderDetailSchema, type OrderCheck, type OrderDetail } from '../model/contracts'
import { orderSearchError, type AttemptSearch } from '../model/order-search'

const invalidResponse = (): ApiError => ({ status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сервера не соответствует опубликованному контракту заказов.', details: {} })
function requestParams(values: HistorySearch | AttemptSearch, attempts = false) {
  return {
    page: values.page, page_size: values.page_size, sort: values.sort,
    ...(values.search && { search: values.search }),
    ...(values.from && { from: values.from }), ...(values.to && { to: values.to }),
    ...(attempts && 'reason_code' in values && values.reason_code && { reason_code: values.reason_code }),
  }
}
function validEnvelope(data: OrderList | OrderAttemptList, values: HistorySearch) {
  return data.page === values.page && data.page_size === values.page_size && data.items.length <= values.page_size && data.items.length <= data.total && new Set(data.items.map((item) => item.id)).size === data.items.length
}
export const orderApi = baseApi.enhanceEndpoints({ addTagTypes: ['Orders', 'OrderAttempts', 'Dishes', 'Inventory', 'Batches', 'Procurement', 'StopList', 'HotOffers'] }).injectEndpoints({
  endpoints: (build) => ({
    checkOrder: build.mutation<OrderCheck, OrderRequest>({
      queryFn: async (body, _api, _options, query) => {
        const valid = orderRequestSchema.safeParse(body)
        if (!valid.success) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Проверьте корзину.', details: {} } }
        const result = await query({ url: '/orders/check', method: 'POST', body: valid.data })
        if (result.error) return { error: result.error }
        const parsed = orderCheckSchema.safeParse(result.data)
        return parsed.success && matchesItems(parsed.data.items, body) && (!parsed.data.can_create || matchesItems(parsed.data.items, body, true)) ? { data: parsed.data } : { error: invalidResponse() }
      },
    }),
    createOrder: build.mutation<OrderDetail, { key: string; body: OrderRequest }>({
      invalidatesTags: (_result, error) => error?.status === 422 ? [] : ['Orders', 'OrderAttempts', 'Inventory', 'Batches', 'Dishes', 'Procurement', 'StopList', 'HotOffers'],
      queryFn: async ({ key, body }, _api, _options, query) => {
        if (!z.uuid().safeParse(key).success || !orderRequestSchema.safeParse(body).success) return { error: { status: 422, code: 'VALIDATION_ERROR', message: 'Проверьте корзину и ключ попытки.', details: {} } }
        const result = await query({ url: '/orders', method: 'POST', body, headers: { 'Idempotency-Key': key } })
        if (result.error) return { error: result.error }
        const parsed = orderDetailSchema.safeParse(result.data)
        return parsed.success && matchesItems(parsed.data.items, body, true) ? { data: parsed.data } : { error: invalidResponse() }
      },
    }),
    orders: build.query<OrderList, HistorySearch>({
      providesTags: ['Orders'],
      queryFn: async (values, _api, _options, query) => {
        const error = orderSearchError(values)
        if (error) return { error: { status: 422, code: 'VALIDATION_ERROR', message: error, details: {} } }
        const result = await query({ url: '/orders', params: requestParams(values) })
        if (result.error) return { error: result.error }
        const parsed = orderListSchema.safeParse(result.data)
        return parsed.success && validEnvelope(parsed.data, values) ? { data: parsed.data } : { error: invalidResponse() }
      },
    }),
    orderAttempts: build.query<OrderAttemptList, AttemptSearch>({
      providesTags: ['OrderAttempts'],
      queryFn: async (values, _api, _options, query) => {
        const error = orderSearchError(values)
        if (error) return { error: { status: 422, code: 'VALIDATION_ERROR', message: error, details: {} } }
        const result = await query({ url: '/order-attempts', params: requestParams(values, true) })
        if (result.error) return { error: result.error }
        const parsed = orderAttemptListSchema.safeParse(result.data)
        return parsed.success && validEnvelope(parsed.data, values) ? { data: parsed.data } : { error: invalidResponse() }
      },
    }),
  }),
})
export const { useOrdersQuery, useOrderAttemptsQuery, useCheckOrderMutation, useCreateOrderMutation } = orderApi
export const invalidateOrders = () => orderApi.util.invalidateTags(['Orders', 'OrderAttempts'])

function matchesItems(items: { dish_id: string; portions: number; recipe_version: number | null }[], body: OrderRequest, versions = false) {
  const merged = new Map<string, OrderRequest['items'][number]>()
  for (const line of body.items) {
    const prior = merged.get(line.dish_id)
    merged.set(line.dish_id, { ...line, portions: (prior?.portions ?? 0) + line.portions, recipe_version: line.recipe_version ?? prior?.recipe_version })
  }
  return items.length === merged.size && new Set(items.map((item) => item.dish_id)).size === items.length && [...merged.values()].every((requested) => items.some((item) => item.dish_id === requested.dish_id && item.portions === requested.portions && (!versions || requested.recipe_version == null || item.recipe_version === requested.recipe_version)))
}
