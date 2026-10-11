import { z } from 'zod'
import { productBriefSchema } from '@/shared/api'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const productSchema = productBriefSchema.extend({
  category: z.object({ id: z.uuid(), name: z.string() }),
  standard_shelf_life_hours: z.number().int().positive().nullable(),
  target_stock: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).nullable(),
  is_active: z.boolean(), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  created_at: z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }),
})
export const productListSchema = z.object({
  items: z.array(productSchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100), meta: z.object({}).passthrough(),
})
export type Product = z.infer<typeof productSchema>
