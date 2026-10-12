import { z } from 'zod'
import type { Dish } from '@/entities/dish'
import { createDishSchema, dishValuesSchema, type DishDraft, type DishValues, type DishFields, type UpdateDish } from '@/shared/api/dish-work'
import { quantityFromInput, quantityToInput } from '@/shared/lib/quantity'

export const emptyDish: DishValues = { name: '', description: '', is_active: true, recipe: [{ product_id: '', quantity_per_portion: '', unit: 'g' }] }
export const dishFormSchema = dishValuesSchema.superRefine((values, ctx) => {
  const add = (path: (string | number)[], message: string) => ctx.addIssue({ code: 'custom', path, message })
  if ([...values.name.trim()].length < 1 || [...values.name.trim()].length > 200) add(['name'], 'Название: от 1 до 200 символов')
  if ([...values.description].length > 2000) add(['description'], 'Описание: не более 2000 символов')
  if (!values.recipe.length) add(['recipe'], 'Добавьте от 1 до 100 ингредиентов')
  const seen = new Set<string>()
  values.recipe.forEach((line, index) => {
    if (!z.uuid().safeParse(line.product_id).success) add(['recipe', index, 'product_id'], 'Выберите продукт')
    if (seen.has(line.product_id.toLowerCase())) add(['recipe', index, 'product_id'], 'Продукт уже есть в рецептуре')
    seen.add(line.product_id.toLowerCase())
    try { quantityFromInput(line.quantity_per_portion, line.unit) } catch (error) { add(['recipe', index, 'quantity_per_portion'], error instanceof Error ? error.message : 'Проверьте расход') }
  })
})
export function dishToValues(dish: Dish): DishValues {
  return { name: dish.name, description: dish.description ?? '', is_active: dish.is_active, recipe: dish.recipe.map((line) => ({ product_id: line.product_id, quantity_per_portion: quantityToInput(line.quantity_per_portion, line.unit)!, unit: line.unit })) }
}
export function dishRecipe(values: DishValues) {
  return values.recipe.map((line) => ({ product_id: line.product_id, quantity_per_portion: quantityFromInput(line.quantity_per_portion, line.unit) }))
}
export function dishFields(values: DishValues): DishFields {
  return createDishSchema.parse({ name: values.name.trim(), description: values.description === '' ? null : values.description, is_active: values.is_active, recipe: dishRecipe(values) })
}
export function dishBaseline(dish: Dish): NonNullable<DishDraft['baseline']> {
  return { revision: dish.revision, fields: { name: dish.name, description: dish.description, is_active: dish.is_active, recipe: dish.recipe.map(({ product_id, quantity_per_portion }) => ({ product_id, quantity_per_portion })) } }
}
export function sameRecipe(a: DishFields['recipe'], b: DishFields['recipe']) {
  return a.length === b.length && a.every((line) => b.some((prior) => prior.product_id.toLowerCase() === line.product_id.toLowerCase() && quantityFromInput(prior.quantity_per_portion, 'g') === quantityFromInput(line.quantity_per_portion, 'g')))
}
export function dishPatch(values: DishValues, baseline: NonNullable<DishDraft['baseline']>): UpdateDish {
  const fields = dishFields(values), result: UpdateDish = { expected_revision: baseline.revision }
  if (fields.name !== baseline.fields.name) result.name = fields.name
  if (fields.description !== baseline.fields.description) result.description = fields.description
  if (fields.is_active !== baseline.fields.is_active) result.is_active = fields.is_active
  if (!sameRecipe(fields.recipe, baseline.fields.recipe)) result.recipe = fields.recipe
  return result
}
/** Maps only published field paths; unknown server details remain in the general error. */
export function dishFieldPath(raw: string): 'name' | 'description' | 'is_active' | `recipe.${number}.product_id` | `recipe.${number}.quantity_per_portion` | undefined {
  const path = raw.replace(/^\/?body[./]/, '').replace(/^\//, '').replace(/\[(\d+)\]/g, '.$1').replaceAll('/', '.')
  return /^(name|description|is_active|recipe\.\d+\.(product_id|quantity_per_portion))$/.test(path) ? path as ReturnType<typeof dishFieldPath> : undefined
}
