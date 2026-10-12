import { z } from 'zod'

// Published product request fields; draft strings intentionally allow invalid unfinished input.
export const productNameSchema = z.string().trim().refine((name) => [...name].length >= 1 && [...name].length <= 200, 'Название: от 1 до 200 символов после удаления пробелов')
export const productFieldsSchema = z.object({
  name: productNameSchema, category_id: z.uuid(), unit: z.enum(['g', 'ml', 'pcs']), display_unit: z.enum(['g', 'kg', 'ml', 'l', 'pcs']),
  standard_shelf_life_hours: z.number().int().min(1).max(2147483647).nullable(),
  target_stock: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).nullable(), is_active: z.boolean(),
})
export function compatibleProductFields(value: { unit: string; display_unit: string; target_stock?: string | null }) {
  return (value.unit === value.display_unit || value.unit === 'g' && value.display_unit === 'kg' || value.unit === 'ml' && value.display_unit === 'l') && (value.unit !== 'pcs' || value.target_stock == null || /^\d+(?:\.0+)?$/.test(value.target_stock))
}
export const productValuesSchema = z.object({
  name: z.string().max(2000), category_id: z.string().max(100), unit: z.enum(['g', 'ml', 'pcs']), display_unit: z.enum(['g', 'kg', 'ml', 'l', 'pcs']),
  standard_shelf_life_hours: z.string().max(100), target_stock: z.string().max(100), is_active: z.boolean(),
})
export const productDraftSchema = z.object({
  resourceId: z.union([z.literal('new'), z.uuid()]), values: productValuesSchema,
  baseline: z.object({ fields: productFieldsSchema, revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER) }).nullable(),
  uncertain: z.boolean(), review: z.boolean(),
})
export type ProductValues = z.infer<typeof productValuesSchema>
export type ProductDraft = z.infer<typeof productDraftSchema>
export type ProductFields = z.infer<typeof productFieldsSchema>
