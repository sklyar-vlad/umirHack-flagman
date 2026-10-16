import { baseApi, type ApiError } from '@/shared/api'
import { shortageReportSchema } from '../model/contracts'
import { shortageSearchError, type ShortageSearch } from '../model/search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ отчёта о недостаче.', details: {} }
export const shortageApi = baseApi.enhanceEndpoints({ addTagTypes: ['IngredientShortages'] }).injectEndpoints({
  endpoints: (build) => ({
    ingredientShortages: build.query<ReturnType<typeof shortageReportSchema.parse>, ShortageSearch>({
      providesTags: ['IngredientShortages'],
      queryFn: async (values, _api, _options, query) => {
        const error = shortageSearchError(values)
        if (error) return { error: { status: 422, code: 'VALIDATION_ERROR', message: error, details: {} } }
        const result = await query({ url: '/analytics/ingredient-shortages', params: {
          page: values.page, page_size: values.page_size,
          ...(values.from && { from: values.from }), ...(values.to && { to: values.to }),
        } })
        if (result.error) return { error: result.error }
        const parsed = shortageReportSchema.safeParse(result.data)
        if (!parsed.success) return { error: invalid }
        const data = parsed.data
        if (data.page !== values.page || data.page_size !== values.page_size || data.items.length > data.page_size || data.items.length > data.total || new Set(data.items.map((row) => `${row.product_id.toLowerCase()}:${row.unit}`)).size !== data.items.length) return { error: invalid }
        return { data }
      },
    }),
  }),
})
export const { useIngredientShortagesQuery } = shortageApi
