import { baseApi, type ApiError } from '@/shared/api'
import { forecastReportSchema } from '../model/contracts'
import { forecastSearchError, type ForecastSearch } from '../model/search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ прогноза стоп-листа.', details: {} }
export const forecastApi = baseApi.enhanceEndpoints({ addTagTypes: ['StopListForecast'] }).injectEndpoints({
  endpoints: (build) => ({
    stopListForecast: build.query<ReturnType<typeof forecastReportSchema.parse>, ForecastSearch>({
      providesTags: ['StopListForecast'],
      queryFn: async (values, _api, _options, query) => {
        const error = forecastSearchError(values)
        if (error) return { error: { status: 422, code: 'VALIDATION_ERROR', message: error, details: {} } }
        const result = await query({ url: '/analytics/stop-list-forecast', params: { lookback_days: values.lookback_days, horizon_hours: values.horizon_hours, page: values.page, page_size: values.page_size } })
        if (result.error) return { error: result.error }
        const parsed = forecastReportSchema.safeParse(result.data)
        if (!parsed.success) return { error: invalid }
        const data = parsed.data
        if (data.page !== values.page || data.page_size !== values.page_size || data.meta.lookback_days !== values.lookback_days || data.meta.horizon_hours !== values.horizon_hours || data.items.length > data.page_size || data.items.length > data.total || new Set(data.items.map((row) => row.dish_id.toLowerCase())).size !== data.items.length) return { error: invalid }
        // Keep the server's estimated_stop_at ASC NULLS LAST order, including ties.
        return { data }
      },
    }),
  }),
})
export const { useStopListForecastQuery } = forecastApi
