import { baseApi, type ApiError } from '@/shared/api'
import type { HistorySearch } from '@/shared/lib/history'
import { orderListSchema, orderAttemptListSchema, type OrderList, type OrderAttemptList } from '../model/contracts'
import { orderSearchError, type AttemptSearch } from '../model/order-search'

const invalidResponse = (): ApiError => ({ status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сервера не соответствует опубликованному контракту истории заказов.', details: {} })
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
export const orderApi = baseApi.enhanceEndpoints({ addTagTypes: ['Orders', 'OrderAttempts'] }).injectEndpoints({
  endpoints: (build) => ({
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
export const { useOrdersQuery, useOrderAttemptsQuery } = orderApi
export const invalidateOrders = () => orderApi.util.invalidateTags(['Orders', 'OrderAttempts'])
