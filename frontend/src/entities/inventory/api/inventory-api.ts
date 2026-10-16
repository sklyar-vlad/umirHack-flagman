import type { z } from 'zod'
import { baseApi, type ApiError } from '@/shared/api'
import { batchesResponseSchema, categoriesResponseSchema, inventoryResponseSchema, type BatchesResponse, type Category, type InventoryResponse } from '../model/contracts'
import type { InventorySearch } from '../model/inventory-search'

function validate<T>(schema: z.ZodType<T>, data: unknown) {
  const parsed = schema.safeParse(data)
  return parsed.success ? { data: parsed.data } : { error: invalidResponse() }
}
function invalidResponse(): ApiError {
  return { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сервера не соответствует опубликованному контракту склада.', details: {} }
}
export interface BatchSearch { productId: string; status?: 'all' | 'valid' | 'expiring' | 'expired' | 'depleted'; sort: 'expires_at' | '-created_at'; page: number; page_size: number }
export const inventoryApi = baseApi.enhanceEndpoints({ addTagTypes: ['Inventory', 'Batches', 'ProductCategories'] }).injectEndpoints({
  endpoints: (build) => ({
    inventory: build.query<InventoryResponse, InventorySearch>({
      providesTags: ['Inventory'],
      queryFn: async (args, _api, _options, query) => {
        const { category_id, search, ...rest } = args
        const result = await query({ url: '/inventory', params: { ...rest, ...(category_id && { category_id }), ...(search && { search }) } })
        return result.error ? { error: result.error } : validate(inventoryResponseSchema, result.data)
      },
    }),
    productBatches: build.query<BatchesResponse, BatchSearch>({
      providesTags: ['Batches'],
      queryFn: async ({ productId, ...params }, _api, _options, query) => {
        const result = await query({ url: `/products/${encodeURIComponent(productId)}/batches`, params })
        if (result.error) return { error: result.error }
        const parsed = validate(batchesResponseSchema, result.data)
        if (parsed.data?.items.some((batch) => batch.product_id !== productId)) return { error: invalidResponse() }
        return parsed
      },
    }),
    productCategories: build.query<Category[], void>({
      providesTags: ['ProductCategories'],
      queryFn: async (_arg, _api, _options, query) => {
        const items: Category[] = []
        for (let page = 1; ; page++) {
          const result = await query({ url: '/product-categories', params: { page, page_size: 100, sort: 'name' } })
          if (result.error) return { error: result.error }
          const parsed = validate(categoriesResponseSchema, result.data)
          if (parsed.error) return parsed
          const data = parsed.data
          if (data.page !== page || data.items.some((item) => items.some((prior) => prior.id === item.id))) return { error: invalidResponse() }
          items.push(...data.items)
          if (items.length >= data.total) return { data: items }
          if (!data.items.length) return { error: invalidResponse() }
        }
      },
    }),
  }),
})
export const { useInventoryQuery, useProductBatchesQuery, useProductCategoriesQuery } = inventoryApi
