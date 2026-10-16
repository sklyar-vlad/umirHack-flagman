import { z } from 'zod'
import { createDishSchema, updateDishSchema, previewDishSchema, recipeInputSchema, type CreateDish, type UpdateDish, type PreviewDish } from '@/shared/api/dish-work'
import { parseQuantity } from '@/shared/lib/quantity'
import { dishReadsChanged } from '../model/dish-changes'
import type { DishSearch } from '../model/dish-search'
import { baseApi, type ApiError } from '@/shared/api'
import { dishSchema, dishListSchema, stopListSchema, type Dish, type DishList, type StopList, dishPreviewSchema, type DishPreview } from '../model/contracts'
import { hotOffersListSchema, type HotOffersList } from '../model/hot-offer-contracts'
import type { HotOfferSearch } from '../model/hot-offer-search'
import type { StopListSearch } from '../model/stop-list-search'

const offerPreferenceSchema = z.object({ is_published:z.boolean(), is_manual:z.boolean(), message:z.string().max(500), revision:z.number().int().nonnegative().safe() })
const invalid = (message = 'Некорректный ответ каталога блюд.'): ApiError => ({ status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message, details: {} })
export const dishApi = baseApi.enhanceEndpoints({ addTagTypes: ['Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'] }).injectEndpoints({ endpoints: (build) => ({
  previewDish: build.mutation<DishPreview, PreviewDish>({
    queryFn: async (body, _api, _options, query) => {
      const request = previewDishSchema.safeParse(body)
      if (!request.success) return { error: { ...invalid(), status: 422, code: 'VALIDATION_ERROR' } }
      const result = await query({ url: '/dishes/preview', method: 'POST', body: request.data })
      if (result.error) return { error: result.error }
      const parsed = dishPreviewSchema.safeParse(result.data)
      return parsed.success && parsed.data.requirements.length === body.recipe.length && body.recipe.every((line) => parsed.data.requirements.some((item) => item.product_id.toLowerCase() === line.product_id.toLowerCase() && recipeInputSchema.shape.quantity_per_portion.safeParse(item.required_quantity).success && parseQuantity(item.required_quantity) === parseQuantity(line.quantity_per_portion))) ? { data: parsed.data } : { error: invalid('Некорректный ответ проверки рецептуры.') }
    },
  }),
  createDish: build.mutation<Dish, CreateDish>({
    invalidatesTags: ['Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'],
    onQueryStarted: async (_body, { dispatch, queryFulfilled }) => { try { await queryFulfilled } catch {  } finally { dispatch(dishReadsChanged()) } },
    queryFn: async (body, _api, _options, query) => {
      const request = createDishSchema.safeParse(body)
      if (!request.success) return { error: { ...invalid(), status: 422, code: 'VALIDATION_ERROR' } }
      const result = await query({ url: '/dishes', method: 'POST', body: request.data })
      if (result.error) return { error: result.error }
      const parsed = dishSchema.safeParse(result.data)
      return parsed.success && matchesDish(parsed.data, request.data) && parsed.data.recipe_version !== null ? { data: parsed.data } : { error: invalid() }
    },
  }),
  updateDish: build.mutation<Dish, { id: string; body: UpdateDish; previousVersion?: number | null }>({
    invalidatesTags: ['Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'],
    onQueryStarted: async (_body, { dispatch, queryFulfilled }) => { try { await queryFulfilled } catch {  } finally { dispatch(dishReadsChanged()) } },
    queryFn: async ({ id, body, previousVersion }, _api, _options, query) => {
      const request = updateDishSchema.safeParse(body)
      if (!request.success) return { error: { ...invalid(), status: 422, code: 'VALIDATION_ERROR' } }
      const result = await query({ url: `/dishes/${encodeURIComponent(id)}`, method: 'PATCH', body: request.data })
      if (result.error) return { error: result.error }
      const parsed = dishSchema.safeParse(result.data)
      return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() && parsed.data.revision > body.expected_revision && matchesDish(parsed.data, request.data) && (previousVersion === undefined || (body.recipe ? parsed.data.recipe_version !== null && parsed.data.recipe_version > (previousVersion ?? 0) : parsed.data.recipe_version === previousVersion)) ? { data: parsed.data } : { error: invalid() }
    },
  }),
  hotOfferPreference: build.query<{ is_published: boolean; is_manual: boolean; message: string; revision: number }, string>({
    providesTags: ['HotOffers'],
    queryFn: async (id,_api,_options,query) => {
      const result = await query(`/hot-offers/${encodeURIComponent(id)}`)
      if (result.error) return { error:result.error }
      const parsed = offerPreferenceSchema.safeParse(result.data)
      return parsed.success ? {data:parsed.data} : {error:invalid('Некорректные настройки предложения.')}
    },
  }),
  updateHotOffer: build.mutation<{ is_published: boolean; is_manual: boolean; message: string; revision: number }, { id: string; is_published: boolean; is_manual: boolean; message: string; expected_revision: number }>({
    queryFn: async ({id,...body},_api,_options,query) => {
      const result = await query({ url:`/hot-offers/${encodeURIComponent(id)}`,method:'PATCH',body })
      if (result.error) return {error:result.error}
      const parsed = offerPreferenceSchema.safeParse(result.data)
      return parsed.success && parsed.data.revision > body.expected_revision && parsed.data.is_published === body.is_published && parsed.data.is_manual === body.is_manual && parsed.data.message === body.message.trim() ? {data:parsed.data} : {error:invalid('Некорректный результат сохранения предложения.')}
    },
    invalidatesTags: ['HotOffers', 'Dashboard'],
  }),
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
export const { useLazyHotOfferPreferenceQuery, useUpdateHotOfferMutation, usePreviewDishMutation, useCreateDishMutation, useUpdateDishMutation, useAdminDishesQuery, useDishesQuery, useLazyDishQuery, useDishQuery, useStopListQuery, useHotOffersQuery } = dishApi
export const useDishCatalogueQuery = useAdminDishesQuery

function matchesDish(dish: Dish, body: Partial<CreateDish>) {
  return (body.name === undefined || dish.name === body.name) && (body.description === undefined || dish.description === body.description) && (body.is_active === undefined || dish.is_active === body.is_active) && (!body.recipe || body.recipe.length === dish.recipe.length && body.recipe.every((line) => dish.recipe.some((item) => item.product_id.toLowerCase() === line.product_id.toLowerCase() && parseQuantity(item.quantity_per_portion) === parseQuantity(line.quantity_per_portion))))
}
