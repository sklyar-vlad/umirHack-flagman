import { z } from 'zod'
import { batchSchema, productBriefSchema } from '@/shared/api'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

const integer = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
export const supplyInstantSchema = z.string().refine((value) => {
  try { parseApiDateTime(value); return true } catch { return false }
}, 'Укажите дату и время с часовым поясом.')
export const supplySummarySchema = z.object({
  id: z.uuid(), number: z.string(), created_at: supplyInstantSchema, received_at: supplyInstantSchema,
  items_count: integer.min(1).max(100), created_by: z.object({ id: z.uuid(), name: z.string() }),
  status: z.literal('received'),
})
export const supplyListSchema = z.object({
  items: z.array(supplySummarySchema), total: integer, page: integer.min(1),
  page_size: integer.min(1).max(100), meta: z.object({}).passthrough(),
})
export type SupplySummary = z.infer<typeof supplySummarySchema>
export type SupplyList = z.infer<typeof supplyListSchema>

export const supplyDetailSchema = supplySummarySchema.extend({
  meta: z.object({ server_time: supplyInstantSchema }).passthrough(),
  items: z.array(z.object({ product: productBriefSchema, batch: batchSchema })).min(1).max(100),
}).superRefine((value, ctx) => {
  const seen = new Set<string>()
  if (value.items.length !== value.items_count) ctx.addIssue({ code: 'custom', message: 'Число исходных строк не совпадает.' })
  for (const { product, batch } of value.items) {
    if (seen.has(batch.id) || batch.product_id !== product.id || batch.unit !== product.unit || batch.supply.id !== value.id || batch.supply.number !== value.number || batch.supply.received_at !== value.received_at) {
      ctx.addIssue({ code: 'custom', message: 'Несогласованные ссылки строки поставки.' })
    }
    seen.add(batch.id)
  }
})
export type SupplyDetail = z.infer<typeof supplyDetailSchema>
export type SupplyLine = SupplyDetail['items'][number]
