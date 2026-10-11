import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

// Published backend/docs/openapi.yaml: InventoryItem, Batch, CategoryListResponse.
const id = z.uuid()
const integer = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const instant = z.string().refine((value) => {
  try { parseApiDateTime(value); return true } catch { return false }
}, 'Ожидается RFC3339 с часовым поясом')
const quantity = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/)
const positiveQuantity = quantity.refine((value) => !/^0+(?:\.0+)?$/.test(value))
const aggregate = z.string().regex(/^\d+\.\d{3}$/)
const unit = z.enum(['g', 'ml', 'pcs'])
const category = z.object({ id, name: z.string() })
const product = z.object({
  id, name: z.string(), unit, display_unit: z.enum(['g', 'kg', 'ml', 'l', 'pcs']),
  category, is_active: z.boolean(),
}).refine((value) => value.unit === value.display_unit || value.unit === 'g' && value.display_unit === 'kg' || value.unit === 'ml' && value.display_unit === 'l', 'Несовместимые единицы')
export const inventoryFilters = ['all', 'expiring', 'expired', 'out_of_stock', 'deficit'] as const
export type InventoryFilter = typeof inventoryFilters[number]
export const inventoryItemSchema = z.object({
  product, physical_quantity: aggregate, available_quantity: aggregate,
  expiring_quantity: aggregate, expired_quantity: aggregate,
  target_stock: quantity.nullable(), deficit_quantity: quantity.nullable(),
  batches_count: integer, nearest_expires_at: instant.nullable(), remaining_seconds: z.number().int().nullable(),
  has_expiring: z.boolean(), has_expired: z.boolean(), is_out_of_stock: z.boolean(),
  status: z.enum(['expired', 'expiring', 'normal', 'out_of_stock']),
}).refine((value) => value.product.unit !== 'pcs' || [value.physical_quantity, value.available_quantity, value.expiring_quantity, value.expired_quantity, value.target_stock, value.deficit_quantity].every((raw) => raw === null || !raw.includes('.') || /^\d+\.0+$/.test(raw)), 'Количество штук должно быть целым')
const envelope = {
  total: integer, page: integer.min(1), page_size: integer.min(1).max(100), meta: z.object({ server_time: instant }).passthrough(),
}
export const inventoryResponseSchema = z.object({
  ...envelope, items: z.array(inventoryItemSchema),
  counts: z.object({ all: integer, expiring: integer, expired: integer, out_of_stock: integer, deficit: integer }),
  attention_batches: z.array(z.object({
    batch_id: id, product_id: id, product_name: z.string(), supply_id: id, supply_number: z.string(),
    line_number: integer.min(1), remaining_quantity: positiveQuantity, unit,
    status: z.enum(['expiring', 'expired']), expires_at: instant, remaining_seconds: z.number().int(),
  })).max(20), attention_total: integer,
})
export const batchSchema = z.object({
  id, product_id: id, unit,
  supply: z.object({ id, number: z.string(), received_at: instant }),
  line_number: integer.min(1), initial_quantity: positiveQuantity, remaining_quantity: quantity,
  produced_at: instant.nullable(), shelf_life_hours: integer.min(1).nullable(),
  expiry_source: z.enum(['explicit', 'calculated']), expires_at: instant, remaining_seconds: z.number().int(),
  status: z.enum(['valid', 'expiring', 'expired', 'depleted']), created_at: instant,
}).refine((value) => value.unit !== 'pcs' || [value.initial_quantity, value.remaining_quantity].every((raw) => !raw.includes('.') || /^\d+\.0+$/.test(raw)), 'Количество штук должно быть целым')
export const batchesResponseSchema = z.object({ ...envelope, items: z.array(batchSchema) })
export const categoriesResponseSchema = z.object({
  ...envelope, meta: z.object({}).passthrough(),
  items: z.array(category.extend({ name: z.string().min(1).max(200), revision: integer.min(1) })),
})
export type InventoryItem = z.infer<typeof inventoryItemSchema>
export type InventoryResponse = z.infer<typeof inventoryResponseSchema>
export type Batch = z.infer<typeof batchSchema>
export type BatchesResponse = z.infer<typeof batchesResponseSchema>
export type Category = z.infer<typeof categoriesResponseSchema>['items'][number]
