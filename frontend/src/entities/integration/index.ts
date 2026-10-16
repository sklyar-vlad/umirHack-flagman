import { z } from 'zod'
import { baseApi, type ApiError } from '@/shared/api'
const instant = z.iso.datetime({ offset: true }), integer = z.number().int().safe(), quantity = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/)
const milli = (raw: string) => { const [whole, fraction = ''] = raw.split('.'); return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3,'0')) }
export const incomingItemSchema = z.object({ id: z.uuid(), product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g','ml','pcs']), quantity, accepted_quantity: quantity, expires_at: instant, line_number: integer.positive() }).refine((line) => milli(line.quantity) > 0n && milli(line.accepted_quantity) <= milli(line.quantity) && (line.unit !== 'pcs' || milli(line.quantity) % 1000n === 0n && milli(line.accepted_quantity) % 1000n === 0n))
export const incomingSchema = z.object({ id: z.uuid(), external_id: z.string(), number: z.string(), source: z.string(), status: z.enum(['pending','partial','received']), revision: integer.positive(), created_at: instant, items: z.array(incomingItemSchema).min(1).max(100), receipt_ids: z.array(z.uuid()) }).refine((doc) => {
  if (new Set(doc.items.map((line) => line.id)).size !== doc.items.length || new Set(doc.items.map((line) => line.line_number)).size !== doc.items.length) return false
  const full = doc.items.every((line) => milli(line.accepted_quantity) === milli(line.quantity)), started = doc.items.some((line) => milli(line.accepted_quantity) > 0n)
  return doc.status === (full ? 'received' : started ? 'partial' : 'pending')
})
const listSchema = z.object({ items: z.array(incomingSchema), total: integer.nonnegative(), page: integer.positive(), page_size: integer.min(1).max(100), meta: z.object({}) })
const tokenSchema = z.object({ id: z.uuid(), name: z.string(), created_at: instant, revoked_at: instant.nullable() }).strict()
const createdTokenSchema = tokenSchema.extend({ token: z.string().regex(/^flagman_[A-Za-z0-9_-]{43}$/) })
export type IncomingSupply = z.infer<typeof incomingSchema>
export type IncomingItem = z.infer<typeof incomingItemSchema>
export interface IncomingSearch { status: 'all'|'pending'|'partial'|'received'; page: number; page_size: number }
const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ сервера интеграций.', details: {} }
export const integrationApi = baseApi.enhanceEndpoints({ addTagTypes: ['IncomingSupplies','IntegrationTokens','Inventory','Batches','Supplies','Dashboard','Dishes','StopList','HotOffers','Procurement','StockMovements','ProductFlow','StopListForecast'] }).injectEndpoints({ endpoints: (build) => ({
  incomingSupplies: build.query<z.infer<typeof listSchema>, IncomingSearch>({
    providesTags: ['IncomingSupplies'],
    queryFn: async (args,_api,_options,query) => {
      const result = await query({ url: '/incoming-supplies', params: { ...args } })
      if (result.error) return { error: result.error }
      const parsed = listSchema.safeParse(result.data)
      return parsed.success && parsed.data.page === args.page && parsed.data.page_size === args.page_size && parsed.data.items.length <= args.page_size && parsed.data.items.length <= parsed.data.total && new Set(parsed.data.items.map((item) => item.id)).size === parsed.data.items.length && parsed.data.items.every((doc) => args.status === 'all' || doc.status === args.status) ? { data: parsed.data } : { error: invalid }
    },
  }),
  acceptIncoming: build.mutation<IncomingSupply, { id: string; expected_revision: number; items: { id: string; quantity: string }[] }>({
    invalidatesTags: ['IncomingSupplies','Inventory','Batches','Supplies','Dashboard','Dishes','StopList','HotOffers','Procurement','StockMovements','ProductFlow','StopListForecast'],
    queryFn: async ({ id,...body },_api,_options,query) => {
      const result = await query({ url: `/incoming-supplies/${encodeURIComponent(id)}/accept`, method: 'POST', body })
      if (result.error) return { error: result.error }
      const parsed = incomingSchema.safeParse(result.data)
      return parsed.success && parsed.data.id === id && parsed.data.revision > body.expected_revision ? { data: parsed.data } : { error: invalid }
    },
  }),
  integrationTokens: build.query<z.infer<typeof tokenSchema>[], void>({
    providesTags: ['IntegrationTokens'],
    queryFn: async (_arg,_api,_options,query) => { const result = await query('/integration-tokens'); if (result.error) return { error: result.error }; const parsed = z.array(tokenSchema).safeParse(result.data); return parsed.success ? { data: parsed.data } : { error: invalid } },
  }),
  createIntegrationToken: build.mutation<z.infer<typeof createdTokenSchema>, { name: string }>({
    invalidatesTags: ['IntegrationTokens'],
    queryFn: async (body,_api,_options,query) => { const result = await query({ url: '/integration-tokens', method: 'POST', body }); if (result.error) return { error: result.error }; const parsed = createdTokenSchema.safeParse(result.data); return parsed.success ? { data: parsed.data } : { error: invalid } },
  }),
  revokeIntegrationToken: build.mutation<void,string>({ query: (id) => ({ url: `/integration-tokens/${encodeURIComponent(id)}`, method: 'DELETE' }), invalidatesTags: ['IntegrationTokens'] }),
}) })
export const { useIncomingSuppliesQuery,useAcceptIncomingMutation,useIntegrationTokensQuery,useCreateIntegrationTokenMutation,useRevokeIntegrationTokenMutation } = integrationApi
