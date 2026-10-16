import { baseApi, type ApiError } from '@/shared/api'
import { lossQuantity, lossReportSchema } from '../model/contracts'
import type { LossSearch } from '../model/search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ отчёта о потерях.', details: {} }
export const lossApi = baseApi.enhanceEndpoints({ addTagTypes: ['WriteOffReport'] }).injectEndpoints({ endpoints: (build) => ({
  writeOffReport: build.query<ReturnType<typeof lossReportSchema.parse>, LossSearch>({
    providesTags: ['WriteOffReport'],
    queryFn: async (values, _api, _options, query) => {
      const result = await query({ url: '/analytics/write-offs', params: Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '')) })
      if (result.error) return { error: result.error }
      const parsed = lossReportSchema.safeParse(result.data)
      if (!parsed.success) return { error: invalid }
      const data = parsed.data
      if (data.page !== values.page || data.page_size !== values.page_size || data.items.length > data.page_size || data.items.length > data.total || new Set(data.items.map((row) => `${row.product_id}:${row.unit}`)).size !== data.items.length) return { error: invalid }
      if (data.items.some((row) => values.product_id && row.product_id !== values.product_id || values.reason === 'expiration' && lossQuantity(row.manual_written_off_quantity) !== 0n || values.reason === 'manual_writeoff' && lossQuantity(row.expired_written_off_quantity) !== 0n)) return { error: invalid }
      return { data }
    },
  }),
}) })
export const { useWriteOffReportQuery } = lossApi
