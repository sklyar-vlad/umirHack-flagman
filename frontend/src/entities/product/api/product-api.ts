import { baseApi, type ApiError } from '@/shared/api'
import { productListSchema, type Product } from '../model/contracts'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ каталога продуктов.', details: {} }
export const productApi = baseApi.enhanceEndpoints({ addTagTypes: ['Products'] }).injectEndpoints({
  endpoints: (build) => ({
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
export const { useActiveProductsQuery } = productApi
