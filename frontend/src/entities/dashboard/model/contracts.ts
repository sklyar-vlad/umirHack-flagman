import { z } from 'zod'
import { attentionBatchSchema } from '@/shared/api'
import { hotOfferSchema, procurementItemSchema } from '@/shared/api/recommendation-contracts'

const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
export const dashboardSchema = z.strictObject({
  expiring_batches_count: count, expired_batches_count: count,
  stop_list_dishes_count: count, out_of_stock_products_count: count,
  attention_total: count, attention_batches: z.array(attentionBatchSchema).max(20),
  hot_offers: z.array(hotOfferSchema).max(3), procurement_recommendations: z.array(procurementItemSchema).max(3),
  settings: z.strictObject({
    restaurant_name: z.string(), timezone: z.string().refine((value) => {
      try { new Intl.DateTimeFormat('ru', { timeZone: value }); return true } catch { return false }
    }), expiring_threshold_hours: count.min(1).max(720),
  }),
  meta: z.object({ server_time: attentionBatchSchema.shape.expires_at }).passthrough(),
}).refine((value) => value.attention_total === value.expiring_batches_count + value.expired_batches_count && value.attention_batches.length <= value.attention_total, 'Несогласованный размер списка внимания')
  .refine((value) => new Set(value.attention_batches.map((item) => item.batch_id.toLowerCase())).size === value.attention_batches.length &&
    new Set(value.hot_offers.map((item) => item.dish.id.toLowerCase())).size === value.hot_offers.length &&
    new Set(value.procurement_recommendations.map((item) => item.product_id.toLowerCase())).size === value.procurement_recommendations.length, 'Повторяющиеся строки обзора')
export type Dashboard = z.infer<typeof dashboardSchema>
