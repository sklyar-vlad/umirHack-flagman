import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

// Published ProductBrief and Batch DTOs, reused across warehouse read endpoints.
const id = z.uuid()
const integer = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const instant = z.string().refine((value) => {
  try { parseApiDateTime(value); return true } catch { return false }
}, 'Ожидается RFC3339 с часовым поясом')
const quantity = z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/)
const positiveQuantity = quantity.refine((value) => !/^0+(?:\.0+)?$/.test(value))
const unit = z.enum(['g', 'ml', 'pcs'])
export const productBriefSchema = z.object({
  id, name: z.string(), unit, display_unit: z.enum(['g', 'kg', 'ml', 'l', 'pcs']),
}).refine((value) => value.unit === value.display_unit || value.unit === 'g' && value.display_unit === 'kg' || value.unit === 'ml' && value.display_unit === 'l', 'Несовместимые единицы')
export const batchSchema = z.object({
  id, product_id: id, unit,
  supply: z.object({ id, number: z.string(), received_at: instant }),
  line_number: integer.min(1), initial_quantity: positiveQuantity, remaining_quantity: quantity,
  produced_at: instant.nullable(), shelf_life_hours: integer.min(1).nullable(),
  expiry_source: z.enum(['explicit', 'calculated']), expires_at: instant, remaining_seconds: z.number().int(),
  status: z.enum(['valid', 'expiring', 'expired', 'depleted']), created_at: instant,
}).refine((value) => value.unit !== 'pcs' || [value.initial_quantity, value.remaining_quantity].every((raw) => !raw.includes('.') || /^\d+\.0+$/.test(raw)), 'Количество штук должно быть целым')
