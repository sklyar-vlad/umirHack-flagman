import { z } from 'zod'
import { baseApi, type ApiError } from '@/shared/api'
import { historyPeriodError, readHistorySearch } from '@/shared/lib/history'
const quantity = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
export const flowRowSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  received_quantity: quantity, ordered_quantity: quantity, expired_quantity: quantity, manual_quantity: quantity, written_off_quantity: quantity,
  movements_count: z.number().int().positive().safe(),
}).refine((row) => {
  const milli = (raw: string) => { const [whole, fraction = ''] = raw.split('.'); return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0')) }
  const amounts = [row.received_quantity, row.ordered_quantity, row.expired_quantity, row.manual_quantity, row.written_off_quantity].map(milli)
  return amounts[2] + amounts[3] === amounts[4] && (row.unit !== 'pcs' || amounts.every((value) => value % 1000n === 0n))
})
const reportSchema = z.object({ items: z.array(flowRowSchema), total: z.number().int().nonnegative().safe(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100), meta: z.object({}) })
export function readFlowSearch(params: URLSearchParams) { const { from, to, page, page_size } = readHistorySearch(params); return { from, to, page, page_size, product_id: params.get('product_id') ?? '' } }
export type FlowSearch = ReturnType<typeof readFlowSearch>
export function flowError(values: FlowSearch) { return historyPeriodError(values) || (values.product_id && !z.uuid().safeParse(values.product_id).success ? 'Проверьте ID продукта.' : null) }
const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный отчёт движения продуктов.', details: {} }
export const flowApi = baseApi.enhanceEndpoints({ addTagTypes: ['ProductFlow'] }).injectEndpoints({ endpoints: (build) => ({
  productFlow: build.query<z.infer<typeof reportSchema>, FlowSearch>({
    providesTags: ['ProductFlow'],
    queryFn: async (values, _api, _options, query) => {
      const result = await query({ url: '/analytics/product-flow', params: Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '')) })
      if (result.error) return { error: result.error }
      const parsed = reportSchema.safeParse(result.data)
      if (!parsed.success) return { error: invalid }
      const data = parsed.data
      return data.page === values.page && data.page_size === values.page_size && data.items.length <= data.page_size && data.items.length <= data.total && new Set(data.items.map((item) => `${item.product_id}:${item.unit}`)).size === data.items.length && data.items.every((item) => !values.product_id || item.product_id === values.product_id) ? { data } : { error: invalid }
    },
  }),
}) })
export const { useProductFlowQuery } = flowApi
