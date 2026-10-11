import { z } from 'zod'
import { formatQuantity } from '@/shared/lib/quantity'

const integer = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const recipeIngredientSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  quantity_per_portion: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).refine((value) => !/^0+(?:\.0+)?$/.test(value)),
}).refine((value) => { try { formatQuantity(value.quantity_per_portion, value.unit); return true } catch { return false } })
export const orderRequestSchema = z.object({ items: z.array(z.object({
  dish_id: z.uuid(), portions: integer.max(1000), recipe_version: integer.nullable().optional(),
}).strict()).min(1).max(100) }).strict().superRefine(({ items }, ctx) => {
  const seen = new Map<string, { portions: number; version?: number | null }>()
  items.forEach((item, index) => {
    const prior = seen.get(item.dish_id)
    if (prior && (prior.portions + item.portions > 1000 || prior.version != null && item.recipe_version != null && prior.version !== item.recipe_version)) ctx.addIssue({ code: 'custom', path: ['items', index], message: 'Конфликт версий или больше 1000 порций блюда.' })
    seen.set(item.dish_id, { portions: (prior?.portions ?? 0) + item.portions, version: item.recipe_version ?? prior?.version })
  })
})
export type OrderRequest = z.infer<typeof orderRequestSchema>
export const basketDishSchema = z.object({ id: z.uuid(), name: z.string(), recipe_version: integer.nullable(), is_active: z.boolean(), recipe: z.array(recipeIngredientSchema).max(100) })
export type BasketDish = z.infer<typeof basketDishSchema>
export const orderDraftSchema = z.object({
  lines: z.array(z.object({ dish: basketDishSchema, portions: z.string().max(20) })).max(100),
  attempt: z.object({ key: z.uuid(), body: orderRequestSchema, startedAt: z.number().int().nonnegative() }).optional(),
}).refine(({ lines }) => new Set(lines.map((line) => line.dish.id)).size === lines.length)
export type OrderDraft = z.infer<typeof orderDraftSchema>

const aggregate = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
export const requirementSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  required_quantity: aggregate, available_quantity: aggregate, missing_quantity: aggregate,
}).refine((value) => { try { for (const raw of [value.required_quantity, value.available_quantity, value.missing_quantity]) formatQuantity(raw, value.unit); return true } catch { return false } })
export const shortageSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  required_quantity: aggregate, available_quantity: aggregate, missing_quantity: aggregate, expired_quantity: aggregate,
  reason_code: z.enum(['ingredient_shortage', 'only_expired_stock']),
}).refine((value) => {
  try { for (const quantity of [value.required_quantity, value.available_quantity, value.missing_quantity, value.expired_quantity]) formatQuantity(quantity, value.unit); return true } catch { return false }
}, 'Количество несовместимо с единицей продукта.')
