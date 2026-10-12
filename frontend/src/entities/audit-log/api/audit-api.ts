import { baseApi, type ApiError } from '@/shared/api'
import { auditListSchema } from '../model/contracts'
import type { AuditSearch } from '../model/search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ журнала действий.', details: {} }
export const auditApi = baseApi.enhanceEndpoints({ addTagTypes: ['AuditLog'] }).injectEndpoints({
  endpoints: (build) => ({
    auditLog: build.query<ReturnType<typeof auditListSchema.parse>, AuditSearch>({
      providesTags: ['AuditLog'],
      queryFn: async (values, _api, _options, query) => {
        const params = Object.fromEntries(Object.entries(values).filter(([, value]) => value !== ''))
        const result = await query({ url: '/audit-log', params })
        if (result.error) return { error: result.error }
        const parsed = auditListSchema.safeParse(result.data)
        if (!parsed.success) return { error: invalid }
        const data = parsed.data
        if (data.page !== values.page || data.page_size !== values.page_size || data.items.length > data.page_size || data.items.length > data.total || new Set(data.items.map((item) => item.id)).size !== data.items.length) return { error: invalid }
        return { data }
      },
    }),
  }),
})
export const { useAuditLogQuery } = auditApi
