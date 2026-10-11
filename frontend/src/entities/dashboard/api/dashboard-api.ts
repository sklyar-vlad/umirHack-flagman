import { baseApi, type ApiError } from '@/shared/api'
import { dashboardSchema, type Dashboard } from '../model/contracts'

export const dashboardApi = baseApi.enhanceEndpoints({ addTagTypes: ['Dashboard', 'Dishes'] }).injectEndpoints({ endpoints: (build) => ({
  dashboard: build.query<Dashboard, void>({
    providesTags: ['Dashboard', 'Dishes'],
    queryFn: async (_arg, _api, _options, query) => {
      const result = await query({ url: '/dashboard' })
      if (result.error) return { error: result.error }
      const parsed = dashboardSchema.safeParse(result.data)
      if (parsed.success) return { data: parsed.data }
      const error: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ обзора не соответствует опубликованному контракту.', details: {} }
      return { error }
    },
  }),
}) })
export const { useDashboardQuery } = dashboardApi
