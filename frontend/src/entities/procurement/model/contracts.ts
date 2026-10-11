import { z } from 'zod'
import { productBriefSchema } from '@/shared/api'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

const quantity = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
const positive = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).refine((value) => !/^0+(?:\.0+)?$/.test(value))
export const procurementItemSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: productBriefSchema.shape.unit, display_unit: productBriefSchema.shape.display_unit,
  available_quantity: quantity, expired_quantity: quantity, target_stock: positive, recommended_quantity: positive,
  reason_codes: z.array(z.enum(['below_target', 'no_valid_stock', 'only_expired_stock'])).min(1).refine((codes) => new Set(codes).size === codes.length),
  reason: z.string(), affected_dishes: z.array(z.object({ id: z.uuid(), name: z.string() })),
}).refine((item) => productBriefSchema.safeParse({ id: item.product_id, name: item.product_name, unit: item.unit, display_unit: item.display_unit }).success &&
  (item.unit !== 'pcs' || [item.available_quantity, item.expired_quantity, item.target_stock, item.recommended_quantity].every((value) => /^\d+(?:\.0+)?$/.test(value))), 'Несовместимые единицы или дробное количество штук')
export const procurementListSchema = z.object({
  items: z.array(procurementItemSchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100),
  meta: z.object({ server_time: z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }) }).passthrough(),
}).refine(({ items }) => new Set(items.map((item) => item.product_id.toLowerCase())).size === items.length, 'Повторяющиеся продукты')
export type ProcurementItem = z.infer<typeof procurementItemSchema>
export type ProcurementList = z.infer<typeof procurementListSchema>
