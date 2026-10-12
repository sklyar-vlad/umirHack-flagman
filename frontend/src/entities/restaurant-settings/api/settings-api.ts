import { baseApi, type ApiError } from '@/shared/api'
import { settingsSchema, updateSettingsSchema, type RestaurantSettings, type UpdateSettings } from '@/shared/api/settings-contracts'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ настроек.', details: {} }
export const settingsApi = baseApi.enhanceEndpoints({ addTagTypes: ['Settings', 'Inventory', 'Batches', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'] }).injectEndpoints({
  endpoints: (build) => ({
    restaurantSettings: build.query<RestaurantSettings, void>({
      // Detached editor revision: background refresh must not replace a dirty form.
      queryFn: async (_arg, _api, _options, query) => {
        const result = await query({ url: '/settings', cache: 'no-store' })
        if (result.error) return { error: result.error }
        const parsed = settingsSchema.safeParse(result.data)
        return parsed.success ? { data: parsed.data } : { error: invalid }
      },
    }),
    updateSettings: build.mutation<RestaurantSettings, UpdateSettings>({
      invalidatesTags: (_result, error) => !error ? ['Settings', 'Inventory', 'Batches', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'] : [],
      queryFn: async (body, _api, _options, query) => {
        const request = updateSettingsSchema.safeParse(body)
        if (!request.success) return { error: { ...invalid, status: 422, code: 'VALIDATION_ERROR' } }
        const result = await query({ url: '/settings', method: 'PATCH', body: request.data })
        if (result.error) return { error: result.error }
        const parsed = settingsSchema.safeParse(result.data)
        return parsed.success && Object.entries(request.data).every(([field, value]) => field === 'expected_revision' ? parsed.data.revision === Number(value) + 1 : parsed.data[field as keyof RestaurantSettings] === value) ? { data: parsed.data } : { error: invalid }
      },
    }),
  }),
})
export const { useLazyRestaurantSettingsQuery, useUpdateSettingsMutation } = settingsApi
export const refreshSettingsViews = () => settingsApi.util.invalidateTags(['Settings', 'Inventory', 'Batches', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'])
