import { baseApi, type ApiError } from '@/shared/api'
import { procurementListSchema, type ProcurementList } from '../model/contracts'
import type { ProcurementSearch } from '../model/procurement-search'
export const procurementApi = baseApi.enhanceEndpoints({ addTagTypes: ['Procurement', 'Dishes'] }).injectEndpoints({ endpoints: (build) => ({
  procurement: build.query<ProcurementList, ProcurementSearch>({
    providesTags: ['Procurement', 'Dishes'],
    queryFn: async ({ category_id, ...args }, _api, _options, query) => {
      const result = await query({ url: '/procurement-recommendations', params: { ...args, ...(category_id && { category_id }) } })
      if (result.error) return { error: result.error }
      const parsed = procurementListSchema.safeParse(result.data)
      if (parsed.success && parsed.data.page === args.page && parsed.data.page_size === args.page_size && parsed.data.items.length <= args.page_size && parsed.data.items.length <= parsed.data.total) return { data: parsed.data }
      const error: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ рекомендаций закупок.', details: {} }
      return { error }
    },
  }),
}) })
export const { useProcurementQuery } = procurementApi
