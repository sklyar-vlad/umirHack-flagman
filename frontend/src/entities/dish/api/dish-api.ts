import { baseApi, type ApiError } from '@/shared/api'
import { dishSchema, dishListSchema, type Dish } from '../model/contracts'

const invalid = (): ApiError => ({ status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ каталога блюд.', details: {} })
export const dishApi = baseApi.enhanceEndpoints({ addTagTypes: ['Dishes'] }).injectEndpoints({ endpoints: (build) => ({
  dishes: build.query<{ items: Dish[]; total: number }, { search: string; page: number }>({
    providesTags: ['Dishes'],
    queryFn: async (args, _api, _options, query) => {
      const result = await query({ url: '/dishes', params: { search: args.search, page: args.page, page_size: 20, sort: 'name', availability: 'all' } })
      if (result.error) return { error: result.error }
      const parsed = dishListSchema.safeParse(result.data)
      return parsed.success && parsed.data.page === args.page && parsed.data.page_size === 20 && parsed.data.items.length <= 20 && parsed.data.items.length <= parsed.data.total ? { data: parsed.data } : { error: invalid() }
    },
  }),
  dish: build.query<Dish, string>({
    providesTags: ['Dishes'],
    queryFn: async (id, _api, _options, query) => {
      const result = await query(`/dishes/${encodeURIComponent(id)}`)
      if (result.error) return { error: result.error }
      const parsed = dishSchema.safeParse(result.data)
      return parsed.success && parsed.data.id === id ? { data: parsed.data } : { error: invalid() }
    },
  }),
}) })
export const { useDishesQuery, useLazyDishQuery } = dishApi
