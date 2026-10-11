import { z } from 'zod'
import { basketDishSchema, shortageSchema } from '@/shared/api/order-work'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

// Published Dish. Quantity strings and server availability are preserved verbatim.
export const dishSchema = basketDishSchema.extend({
  description: z.string().nullable(), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  is_available: z.boolean(), available_portions: z.string().regex(/^\d+$/),
  shortages: z.array(shortageSchema),
  unavailability_reasons: z.array(z.enum(['ingredient_shortage', 'only_expired_stock', 'recipe_missing', 'dish_inactive'])),
  meta: z.object({ server_time: z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }) }).passthrough(),
})
export const dishListSchema = z.object({ items: z.array(dishSchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100), meta: z.object({}).passthrough() })
export type Dish = z.infer<typeof dishSchema>
export const stopListSchema = dishListSchema.extend({ meta: dishSchema.shape.meta }).refine(({ items }) =>
  items.every((dish) => dish.is_active && !dish.is_available && !dish.unavailability_reasons.includes('dish_inactive')) && new Set(items.map((dish) => dish.id.toLowerCase())).size === items.length,
  'Стоп-лист содержит только активные недоступные блюда.',
)
export type StopList = z.infer<typeof stopListSchema>
