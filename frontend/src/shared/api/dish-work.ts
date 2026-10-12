import { z } from 'zod'
import { productNameSchema } from './product-work'

export const recipeInputSchema = z.object({
  product_id: z.uuid(),
  quantity_per_portion: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).refine((value) => !/^0+(?:\.0+)?$/.test(value), 'Расход должен быть положительным'),
}).strict()
export const recipeInputsSchema = z.array(recipeInputSchema).min(1).max(100).superRefine((items, ctx) => {
  const seen = new Set<string>()
  items.forEach((item, index) => {
    const id = item.product_id.toLowerCase()
    if (seen.has(id)) ctx.addIssue({ code: 'custom', path: [index, 'product_id'], message: 'Продукт уже есть в рецептуре' })
    seen.add(id)
  })
})
const description = z.string().refine((value) => [...value].length <= 2000, 'Описание: не более 2000 символов').nullable()
export const dishFieldsSchema = z.object({ name: productNameSchema, description, is_active: z.boolean(), recipe: recipeInputsSchema })
export const createDishSchema = dishFieldsSchema.strict()
export const updateDishSchema = dishFieldsSchema.partial().extend({ expected_revision: z.number().int().min(1).max(2147483646) }).strict().refine((value) => Object.keys(value).length > 1, 'Нет изменений')
export const previewDishSchema = z.object({ recipe: recipeInputsSchema }).strict()
export const dishValuesSchema = z.object({
  name: z.string().max(2000), description: z.string().max(10000), is_active: z.boolean(),
  recipe: z.array(z.object({ product_id: z.string().max(100), quantity_per_portion: z.string().max(100), unit: z.enum(['g', 'ml', 'pcs']) })).max(100),
})
export const dishDraftSchema = z.object({
  resourceId: z.union([z.literal('new'), z.uuid()]), values: dishValuesSchema,
  baseline: z.object({ fields: dishFieldsSchema.extend({ recipe: z.array(recipeInputSchema).max(100) }), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER) }).nullable(),
  uncertain: z.boolean(), review: z.boolean(),
})
export type DishValues = z.infer<typeof dishValuesSchema>
export type DishDraft = z.infer<typeof dishDraftSchema>
export type DishFields = z.infer<typeof dishFieldsSchema>
export type CreateDish = z.infer<typeof createDishSchema>
export type UpdateDish = z.infer<typeof updateDishSchema>
export type PreviewDish = z.infer<typeof previewDishSchema>
