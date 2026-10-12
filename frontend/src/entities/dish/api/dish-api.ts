import type { DishSearch } from '../model/dish-search'
import { baseApi, type ApiError } from '@/shared/api'
import { dishSchema, dishListSchema, stopListSchema, type Dish, type DishList, type StopList } from '../model/contracts'
import { hotOffersListSchema, type HotOffersList } from '../model/hot-offer-contracts'
import type { HotOfferSearch } from '../model/hot-offer-search'
import type { StopListSearch } from '../model/stop-list-search'

const invalid = (message = 'Некорректный ответ каталога блюд.'): ApiError => ({ status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message, details: {} })
export const dishApi = baseApi.enhanceEndpoints({ addTagTypes: ['Dishes', 'StopList', 'HotOffers'] }).injectEndpoints({ endpoints: (build) => ({
  hotOffers: build.query<HotOffersList, HotOfferSearch>({
    providesTags: ['HotOffers', 'Dishes'],
    queryFn: async (args, _api, _options, query) => {
      const result = await query({ url: '/hot-offers', params: { ...args } })
      if (result.error) return { error: result.error }
      const parsed = hotOffersListSchema.safeParse(result.data)
      return parsed.success && parsed.data.page === args.page && parsed.data.page_size === args.page_size && parsed.data.items.length <= args.page_size && parsed.data.items.length <= parsed.data.total ? { data: parsed.data } : { error: invalid('Некорректный ответ горячих предложений.') }
    },
  }),
  stopList: build.query<StopList, StopListSearch>({
    providesTags: ['StopList', 'Dishes'],
    queryFn: async (args, _api, _options, query) => {
      const result = await query({ url: '/stop-list', params: { ...args, sort: 'name' } })
      if (result.error) return { error: result.error }
      const parsed = stopListSchema.safeParse(result.data)
      return parsed.success && parsed.data.page === args.page && parsed.data.page_size === args.page_size && parsed.data.items.length <= args.page_size && parsed.data.items.length <= parsed.data.total ? { data: parsed.data } : { error: invalid('Некорректный ответ стоп-листа.') }
    },
  }),
  adminDishes: build.query<DishList, DishSearch>({
    providesTags: ['Dishes'],
    queryFn: async ({ search, ...params }, _api, _options, query) => {
      const result = await query({ url: '/dishes', params: { ...params, ...(search && { search }) } })
      if (result.error) return { error: result.error }
      const parsed = dishListSchema.safeParse(result.data)
      if (!parsed.success) return { error: invalid() }
      const data = parsed.data
      if (data.page !== params.page || data.page_size !== params.page_size || data.items.length > params.page_size || data.items.length > data.total || new Set(data.items.map((dish) => dish.id.toLowerCase())).size !== data.items.length || data.items.some((dish) => params.is_active !== 'all' && dish.is_active !== (params.is_active === 'true') || params.availability !== 'all' && dish.is_available !== (params.availability === 'available'))) return { error: invalid() }
      return { data }
    },
  }),
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
      return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalid() }
    },
  }),
}) })
export const { useAdminDishesQuery, useDishesQuery, useLazyDishQuery, useDishQuery, useStopListQuery, useHotOffersQuery } = dishApi
