import { baseApi, type ApiError } from '@/shared/api'
import { supplyListSchema, type SupplyList } from '../model/contracts'
import { supplyPeriodError, type SupplySearch } from '../model/supply-search'

function invalidResponse(): ApiError {
  return { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сервера не соответствует опубликованному контракту поставок.', details: {} }
}
export const supplyApi = baseApi.enhanceEndpoints({ addTagTypes: ['Supplies'] }).injectEndpoints({
  endpoints: (build) => ({
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
export const { useSuppliesQuery } = supplyApi
/** Dispatch after a successful reception, including when its response is replayed. */
export const invalidateSupplies = () => supplyApi.util.invalidateTags(['Supplies'])
