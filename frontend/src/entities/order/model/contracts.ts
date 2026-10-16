import { requirementSchema, shortageSchema } from '@/shared/api/order-work'
export { requirementSchema, shortageSchema } from '@/shared/api/order-work'
import { formatQuantity } from '@/shared/lib/quantity'
import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

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

export const orderCheckSchema = z.object({
  can_create: z.boolean(), items: z.array(orderItemSchema).min(1).max(100),
  requirements: z.array(requirementSchema), shortages: z.array(shortageSchema), issues: z.array(orderIssueSchema), meta: z.object({ server_time: instant }).passthrough(),
}).refine((value) => !value.can_create || value.shortages.length === 0 && value.issues.length === 0 && value.items.every((item) => item.recipe_version !== null) && value.requirements.every((item) => /^0+(?:\.0+)?$/.test(item.missing_quantity)))
const quantity = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/)
export const allocationSchema = z.object({
  movement_id: z.uuid(), product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  batch_id: z.uuid(), supply_id: z.uuid(), supply_number: z.string(), line_number: integer.min(1),
  quantity: quantity.refine((value) => !/^0+(?:\.0+)?$/.test(value)), batch_remaining_quantity_after: quantity,
}).refine((value) => { try { formatQuantity(value.quantity, value.unit); formatQuantity(value.batch_remaining_quantity_after, value.unit); return true } catch { return false } })
export const orderDetailSchema = z.object({
  id: z.uuid(), number: z.string().regex(/^\d+$/), status: z.literal('completed'), created_at: instant, created_by: actor,
  items: z.array(orderItemSchema.extend({ id: z.uuid(), recipe_version: integer.min(1), allocations: z.array(allocationSchema).min(1) })).min(1).max(100),
})
export const orderRefusalSchema = z.object({ attempt_id: z.uuid(), requirements: z.array(requirementSchema), shortages: z.array(shortageSchema), issues: z.array(orderIssueSchema) })
export type OrderCheck = z.infer<typeof orderCheckSchema>
export type OrderDetail = z.infer<typeof orderDetailSchema>
export type OrderRefusal = z.infer<typeof orderRefusalSchema>
export type Requirement = z.infer<typeof requirementSchema>
