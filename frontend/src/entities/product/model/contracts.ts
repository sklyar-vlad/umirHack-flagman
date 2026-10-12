import { z } from 'zod'
import { productBriefSchema } from '@/shared/api'
import { productFieldsSchema, compatibleProductFields } from '@/shared/api/product-work'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const productSchema = productBriefSchema.extend({
  category: z.object({ id: z.uuid(), name: z.string() }),
  standard_shelf_life_hours: z.number().int().positive().nullable(),
  target_stock: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).nullable(),
  is_active: z.boolean(), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  created_at: z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }),
}).refine((value) => value.unit !== 'pcs' || value.target_stock === null || /^\d+(?:\.0+)?$/.test(value.target_stock), 'Целевой запас штук должен быть целым')
export const productListSchema = z.object({
  items: z.array(productSchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100), meta: z.object({}).passthrough(),
})
export type Product = z.infer<typeof productSchema>

export type ProductList = z.infer<typeof productListSchema>

export const createProductSchema = productFieldsSchema.partial({ standard_shelf_life_hours: true, target_stock: true, is_active: true }).strict().refine(compatibleProductFields, 'Несовместимые единицы или дробные штуки')
export const updateProductSchema = productFieldsSchema.partial().extend({ expected_revision: z.number().int().min(1).max(2147483646) }).strict().refine((value) => Object.keys(value).length > 1, 'Передайте изменённые поля').refine((value) => !(value.unit && value.display_unit) || compatibleProductFields({ ...value, unit: value.unit!, display_unit: value.display_unit! }), 'Несовместимые единицы или дробные штуки')
export type CreateProduct = z.infer<typeof createProductSchema>
export type UpdateProduct = z.infer<typeof updateProductSchema>
