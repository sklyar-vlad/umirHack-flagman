import { z } from 'zod'
import { parseMovementQuantity, parseQuantity } from '@/shared/lib/quantity'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const movementNames = { receipt: 'Приход', order_consumption: 'Расход заказа', manual_writeoff: 'Ручное списание', expiration: 'Автоматическое списание' }
export const movementTypeSchema = z.enum(['receipt', 'order_consumption', 'manual_writeoff', 'expiration'])
const instant = z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } })
const signed = z.string().regex(/^-?\d{1,11}(?:\.\d{1,3})?$/), quantity = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/)
export const movementSchema = z.object({
  id: z.uuid(), batch_id: z.uuid(), product_id: z.uuid(), order_id: z.uuid().nullable(), order_item_id: z.uuid().nullable(), type: movementTypeSchema,
  delta_quantity: signed, remaining_quantity_after: quantity, product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  actor: z.object({ id: z.uuid(), name: z.string() }).strict().nullable(), occurred_at: instant, note: z.string().nullable(),
}).strict().refine((value) => {
  try {
    const delta = parseMovementQuantity(value.delta_quantity)!, remaining = parseQuantity(value.remaining_quantity_after)!
    return (value.type === 'receipt' ? delta > 0n : delta < 0n) &&
      (value.unit !== 'pcs' || delta % 1000n === 0n && remaining % 1000n === 0n) &&
      (value.type === 'order_consumption' ? !!value.order_id && !!value.order_item_id : value.order_id === null && value.order_item_id === null)
  } catch { return false }
}, 'Некорректные знаки, единицы или связи исторического движения')
const integer = z.number().int().safe()
export const movementsSchema = z.object({ items: z.array(movementSchema), total: integer.nonnegative(), page: integer.positive(), page_size: integer.min(1).max(100), meta: z.object({}) }).strict()
export type StockMovement = z.infer<typeof movementSchema>
