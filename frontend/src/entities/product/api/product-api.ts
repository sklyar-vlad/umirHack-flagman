import { parseQuantity } from '@/shared/lib/quantity'
import { baseApi, type ApiError } from '@/shared/api'
import { productListSchema, productSchema, createProductSchema, updateProductSchema, type CreateProduct, type UpdateProduct, type Product, type ProductList } from '../model/contracts'

import type { ProductSearch } from '../model/product-search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ каталога продуктов.', details: {} }
function matchesFields(product: Product, body: CreateProduct | UpdateProduct) {
  return Object.entries(body).every(([field, value]) => {
    if (field === 'expected_revision') return product.revision === Number(value) + 1
    if (field === 'category_id') return product.category.id.toLowerCase() === String(value).toLowerCase()
    if (field === 'target_stock') return parseQuantity(product.target_stock) === parseQuantity(value as string | null)
    return product[field as keyof Product] === value
  })
}
export const productApi = baseApi.enhanceEndpoints({ addTagTypes: ['Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'] }).injectEndpoints({
  endpoints: (build) => ({
    product: build.query<Product, string>({
      queryFn: async (id, _api, _options, query) => {
        const result = await query(`/products/${encodeURIComponent(id)}`)
        if (result.error) return { error: result.error }
        const parsed = productSchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalid }
      },
    }),
    createProduct: build.mutation<Product, CreateProduct>({
      invalidatesTags: ['Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'],
      queryFn: async (body, _api, _options, query) => {
        const request = createProductSchema.safeParse(body)
        if (!request.success) return { error: { ...invalid, status: 422, code: 'VALIDATION_ERROR' } }
        const result = await query({ url: '/products', method: 'POST', body: request.data })
        if (result.error) return { error: result.error }
        const parsed = productSchema.safeParse(result.data)
        return parsed.success && matchesFields(parsed.data, request.data) ? { data: parsed.data } : { error: invalid }
      },
    }),
    updateProduct: build.mutation<Product, { id: string; body: UpdateProduct }>({
      invalidatesTags: ['Products', 'Inventory', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Dashboard', 'StopListForecast'],
      queryFn: async ({ id, body }, _api, _options, query) => {
        const request = updateProductSchema.safeParse(body)
        if (!request.success) return { error: { ...invalid, status: 422, code: 'VALIDATION_ERROR' } }
        const result = await query({ url: `/products/${encodeURIComponent(id)}`, method: 'PATCH', body: request.data })
        if (result.error) return { error: result.error }
        const parsed = productSchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() && matchesFields(parsed.data, request.data) ? { data: parsed.data } : { error: invalid }
      },
    }),
    products: build.query<ProductList, ProductSearch>({
      providesTags: ['Products'],
      queryFn: async ({ search, category_id, ...params }, _api, _options, query) => {
        const result = await query({ url: '/products', params: { ...params, ...(search && { search }), ...(category_id && { category_id }) } })
        if (result.error) return { error: result.error }
        const parsed = productListSchema.safeParse(result.data)
        if (!parsed.success || parsed.data.page !== params.page || parsed.data.page_size !== params.page_size || parsed.data.items.length > params.page_size || parsed.data.items.length > parsed.data.total || new Set(parsed.data.items.map((item) => item.id)).size !== parsed.data.items.length) return { error: invalid }
        return { data: parsed.data }
      },
    }),
    activeProducts: build.query<Product[], void>({
      providesTags: ['Products'],
      queryFn: async (_arg, _api, _options, query) => {
        const items: Product[] = []
        for (let page = 1; ; page++) {
          const result = await query({ url: '/products', params: { is_active: 'true', sort: 'name', page, page_size: 100 } })
          if (result.error) return { error: result.error }
          const parsed = productListSchema.safeParse(result.data)
          if (!parsed.success) return { error: invalid }
          const data = parsed.data
          if (data.page !== page || data.page_size !== 100 || data.items.length > 100 || data.items.some((product) => !product.is_active || items.some((prior) => prior.id === product.id)) || new Set(data.items.map((product) => product.id)).size !== data.items.length || items.length + data.items.length > data.total) return { error: invalid }
          items.push(...data.items)
          if (items.length === data.total) return { data: items }
          if (!data.items.length) return { error: invalid }
        }
      },
    }),
  }),
})
export const { useActiveProductsQuery, useProductsQuery, useLazyProductQuery, useCreateProductMutation, useUpdateProductMutation } = productApi
