import { baseApi, type ApiError } from '@/shared/api'
import { categorySchema, categoryListSchema, createCategorySchema, updateCategorySchema, type Category, type CategoryList, type CategorySearch, type CreateCategory, type UpdateCategory } from '../model/contracts'
const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ категорий.', details: {} }
const invalidRequest: ApiError = { status: 422, code: 'VALIDATION_ERROR', message: 'Проверьте название и ревизию категории.', details: {} }
export const categoryApi = baseApi.enhanceEndpoints({ addTagTypes: ['ProductCategories', 'Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'] }).injectEndpoints({
  endpoints: (build) => ({
    categories: build.query<CategoryList, CategorySearch>({
      providesTags: ['ProductCategories'],
      queryFn: async ({ search, ...params }, _api, _options, query) => {
        const result = await query({ url: '/product-categories', params: { ...params, ...(search && { search }) } })
        if (result.error) return { error: result.error }
        const parsed = categoryListSchema.safeParse(result.data)
        if (!parsed.success || parsed.data.page !== params.page || parsed.data.page_size !== params.page_size || parsed.data.items.length > params.page_size || parsed.data.items.length > parsed.data.total || new Set(parsed.data.items.map((item) => item.id)).size !== parsed.data.items.length) return { error: invalid }
        return { data: parsed.data }
      },
    }),
    category: build.query<Category, string>({
      // An editor owns a fresh revision. List invalidation must not replace a dirty edit.

      queryFn: async (id, _api, _options, query) => {
        const result = await query(`/product-categories/${encodeURIComponent(id)}`)
        if (result.error) return { error: result.error }
        const parsed = categorySchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalid }
      },
    }),
    createCategory: build.mutation<Category, CreateCategory>({
      // Even an indeterminate write may have reached the server; refresh reads, never auto-retry it.
      invalidatesTags: ['ProductCategories', 'Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'],
      queryFn: async (body, _api, _options, query) => {
        const request = createCategorySchema.safeParse(body)
        if (!request.success) return { error: invalidRequest }
        const result = await query({ url: '/product-categories', method: 'POST', body: request.data })
        if (result.error) return { error: result.error }
        const parsed = categorySchema.safeParse(result.data)
        return parsed.success ? { data: parsed.data } : { error: invalid }
      },
    }),
    updateCategory: build.mutation<Category, { id: string; body: UpdateCategory }>({
      invalidatesTags: ['ProductCategories', 'Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard'],
      queryFn: async ({ id, body }, _api, _options, query) => {
        const request = updateCategorySchema.safeParse(body)
        if (!request.success) return { error: invalidRequest }
        const result = await query({ url: `/product-categories/${encodeURIComponent(id)}`, method: 'PATCH', body: request.data })
        if (result.error) return { error: result.error }
        const parsed = categorySchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalid }
      },
    }),
  }),
})
export const { useCategoriesQuery, useCategoryQuery, useLazyCategoryQuery, useCreateCategoryMutation, useUpdateCategoryMutation } = categoryApi
