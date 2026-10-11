import { z } from 'zod'
import { dishSchema } from './contracts'
import { formatQuantity } from '@/shared/lib/quantity'

const positive = z.string().regex(/^\d{1,11}\.\d{3}$/).refine((value) => !/^0+\.0+$/.test(value))
const integer = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const hotOfferBatchSchema = z.object({
  batch_id: z.uuid(), product_id: z.uuid(), product_name: z.string(), supply_id: z.uuid(), supply_number: z.string(),
  line_number: integer, unit: z.enum(['g', 'ml', 'pcs']), remaining_quantity: positive, used_for_one_portion: positive,
  expires_at: dishSchema.shape.meta.shape.server_time, remaining_seconds: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
}).refine((value) => {
  try { formatQuantity(value.remaining_quantity, value.unit); formatQuantity(value.used_for_one_portion, value.unit); return true } catch { return false }
}, 'Количество несовместимо с единицей продукта.')
export const hotOfferSchema = z.object({
  dish: z.object({ id: z.uuid(), name: z.string(), recipe_version: integer }), rank: integer,
  priority_score: z.string().regex(/^\d+\.\d{4}$/).refine((value) => Number(value) <= 100),
  priority_level: z.enum(['high', 'medium', 'low']), available_portions: z.string().regex(/^[1-9]\d*$/), reason: z.string(),
  problem_batches: z.array(hotOfferBatchSchema).min(1),
})
export const hotOffersListSchema = z.object({
  items: z.array(hotOfferSchema), total: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
  page: integer, page_size: integer.max(100), meta: dishSchema.shape.meta,
}).refine(({ items }) => new Set(items.map((item) => item.dish.id.toLowerCase())).size === items.length, 'Повторяющееся предложение.')
export type HotOffer = z.infer<typeof hotOfferSchema>
export type HotOffersList = z.infer<typeof hotOffersListSchema>
