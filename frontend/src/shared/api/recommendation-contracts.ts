import { z } from 'zod'
import { attentionBatchSchema, productBriefSchema } from './warehouse-contracts'
import { formatQuantity } from '@/shared/lib/quantity'

// Published recommendation DTOs shared by individual reads and DashboardResponse.
const positive = z.string().regex(/^\d{1,11}\.\d{3}$/).refine((value) => !/^0+\.0+$/.test(value))
const integer = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const hotOfferBatchSchema = z.object({
  batch_id: z.uuid(), product_id: z.uuid(), product_name: z.string(), supply_id: z.uuid(), supply_number: z.string(),
  line_number: integer, unit: z.enum(['g', 'ml', 'pcs']), remaining_quantity: positive, used_for_one_portion: positive,
  expires_at: attentionBatchSchema.shape.expires_at, remaining_seconds: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
}).refine((value) => {
  try { formatQuantity(value.remaining_quantity, value.unit); formatQuantity(value.used_for_one_portion, value.unit); return true } catch { return false }
}, 'Количество несовместимо с единицей продукта.')
export const hotOfferSchema = z.object({
  dish: z.object({ id: z.uuid(), name: z.string(), recipe_version: integer }), rank: integer,
  priority_score: z.string().regex(/^\d+\.\d{4}$/).refine((value) => Number(value) <= 100),
  priority_level: z.enum(['high', 'medium', 'low']), available_portions: z.string().regex(/^[1-9]\d*$/), reason: z.string(),
  problem_batches: z.array(hotOfferBatchSchema).min(1),
})
const quantity = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
const positiveTarget = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).refine((value) => !/^0+(?:\.0+)?$/.test(value))
export const procurementItemSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: productBriefSchema.shape.unit, display_unit: productBriefSchema.shape.display_unit,
  available_quantity: quantity, expired_quantity: quantity, target_stock: positiveTarget, recommended_quantity: positiveTarget,
  reason_codes: z.array(z.enum(['below_target', 'no_valid_stock', 'only_expired_stock'])).min(1).refine((codes) => new Set(codes).size === codes.length),
  reason: z.string(), affected_dishes: z.array(z.object({ id: z.uuid(), name: z.string() })),
}).refine((item) => productBriefSchema.safeParse({ id: item.product_id, name: item.product_name, unit: item.unit, display_unit: item.display_unit }).success &&
  (item.unit !== 'pcs' || [item.available_quantity, item.expired_quantity, item.target_stock, item.recommended_quantity].every((value) => /^\d+(?:\.0+)?$/.test(value))), 'Несовместимые единицы или дробное количество штук')
