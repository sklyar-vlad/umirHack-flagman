import { z } from 'zod'
import { formatQuantity } from '@/shared/lib/quantity'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

// Published OrderSummary, OrderAttempt and their historical snapshots.
const integer = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const instant = z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } })
const actor = z.object({ id: z.uuid(), name: z.string() })
export const orderReasonSchema = z.enum(['INSUFFICIENT_STOCK', 'RECIPE_CHANGED', 'DISH_UNAVAILABLE'])
export type OrderReason = z.infer<typeof orderReasonSchema>
export const orderReasons: Record<OrderReason, string> = {
  INSUFFICIENT_STOCK: 'Недостаточно продуктов', RECIPE_CHANGED: 'Рецептура изменилась', DISH_UNAVAILABLE: 'Блюдо недоступно',
}
export const orderItemSchema = z.object({
  dish_id: z.uuid(), dish_name: z.string(), portions: integer.min(1).max(1000), recipe_version: integer.min(1).nullable(),
})
const aggregate = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
export const shortageSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  required_quantity: aggregate, available_quantity: aggregate, missing_quantity: aggregate, expired_quantity: aggregate,
  reason_code: z.enum(['ingredient_shortage', 'only_expired_stock']),
}).refine((value) => {
  try { for (const quantity of [value.required_quantity, value.available_quantity, value.missing_quantity, value.expired_quantity]) formatQuantity(quantity, value.unit); return true } catch { return false }
}, 'Количество несовместимо с единицей продукта.')
export const orderIssueSchema = z.object({
  dish_id: z.uuid(), code: z.enum(['dish_inactive', 'recipe_missing', 'recipe_changed']), message: z.string(), current_recipe_version: integer.min(1).nullable(),
})
export const orderSummarySchema = z.object({
  id: z.uuid(), number: z.string(), status: z.literal('completed'), created_at: instant, created_by: actor,
  items: z.array(orderItemSchema), total_portions: integer.min(1).max(100000),
})
export const orderAttemptSchema = z.object({
  id: z.uuid(), created_at: instant, created_by: actor, reason_code: orderReasonSchema,
  items: z.array(orderItemSchema), shortages: z.array(shortageSchema), issues: z.array(orderIssueSchema),
})
const envelope = z.object({ total: integer, page: integer.min(1), page_size: integer.min(1).max(100), meta: z.object({}).passthrough() })
export const orderListSchema = envelope.extend({ items: z.array(orderSummarySchema) })
export const orderAttemptListSchema = envelope.extend({ items: z.array(orderAttemptSchema) })
export type OrderItem = z.infer<typeof orderItemSchema>
export type OrderSummary = z.infer<typeof orderSummarySchema>
export type OrderAttempt = z.infer<typeof orderAttemptSchema>
export type OrderList = z.infer<typeof orderListSchema>
export type OrderAttemptList = z.infer<typeof orderAttemptListSchema>
