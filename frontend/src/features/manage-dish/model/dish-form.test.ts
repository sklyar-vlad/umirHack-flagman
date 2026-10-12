import { describe, expect, it } from 'vitest'
import { createDishSchema, updateDishSchema, dishDraftSchema } from '@/shared/api/dish-work'
import { dishCatalog } from '@/app/mocks/dish-catalog-fixtures'
import { dishBaseline, dishFields, dishFieldPath, dishFormSchema, dishPatch, dishToValues, emptyDish, sameRecipe } from './dish-form'
const id = '11111111-1111-4111-8111-111111111111'
const values = { ...emptyDish, name: ' Суп ', recipe: [{ product_id: id, quantity_per_portion: '0,001', unit: 'g' as const }] }
describe('F20 exact recipe form and published writes', () => {
  it('trims the name, retains description and converts comma exactly; empty description is null', () => {
    expect(dishFormSchema.safeParse(values).success).toBe(true)
    expect(dishFields(values)).toEqual({ name: 'Суп', description: null, is_active: true, recipe: [{ product_id: id, quantity_per_portion: '0.001' }] })
    expect(dishFields({ ...values, description: ' Текст ' }).description).toBe(' Текст ')
  })
  it.each(['0', '-1', '1.0001', '100000000000', 'NaN', '1e3'])('rejects invalid positive consumption %s', (quantity_per_portion) => {
    expect(dishFormSchema.safeParse({ ...values, recipe: [{ ...values.recipe[0], quantity_per_portion }] }).success).toBe(false)
  })
  it('checks pcs integers, exact upper boundary, Unicode lengths and active-ID format', () => {
    expect(dishFormSchema.safeParse({ ...values, recipe: [{ ...values.recipe[0], unit: 'pcs' }] }).success).toBe(false)
    expect(dishFormSchema.safeParse({ ...values, recipe: [{ ...values.recipe[0], unit: 'pcs', quantity_per_portion: '1.000' }] }).success).toBe(true)
    expect(dishFields({ ...values, recipe: [{ ...values.recipe[0], quantity_per_portion: '99999999999.999' }] }).recipe[0].quantity_per_portion).toBe('99999999999.999')
    expect(dishFormSchema.safeParse({ ...values, name: '😀'.repeat(200), description: '😀'.repeat(2000) }).success).toBe(true)
    expect(dishFormSchema.safeParse({ ...values, name: '😀'.repeat(201) }).success).toBe(false)
    expect(dishFormSchema.safeParse({ ...values, description: '😀'.repeat(2001) }).success).toBe(false)
    expect(dishFormSchema.safeParse({ ...values, recipe: [{ ...values.recipe[0], product_id: '' }] }).success).toBe(false)
  })
  it('rejects case-alias duplicate UUIDs and recipes outside 1–100', () => {
    const line = { ...values.recipe[0], product_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }
    expect(dishFormSchema.safeParse({ ...values, recipe: [line, { ...line, product_id: line.product_id.toUpperCase() }] }).success).toBe(false)
    expect(createDishSchema.safeParse({ ...dishFields(values), recipe: [] }).success).toBe(false)
    const recipe = Array.from({ length: 100 }, (_, i) => ({ ...line, product_id: `00000000-0000-4000-8000-${String(i).padStart(12, '0')}` }))
    expect(dishFormSchema.safeParse({ ...values, recipe }).success).toBe(true)
    expect(dishFormSchema.safeParse({ ...values, recipe: [...recipe, line] }).success).toBe(false)
  })
  it('metadata-only PATCH omits recipe; changed recipe sends all rows with expected_revision', () => {
    const dish = dishCatalog()[0], original = dishToValues(dish), baseline = dishBaseline(dish)
    expect(dishPatch({ ...original, name: 'Другое имя', description: '' }, baseline)).toEqual({ expected_revision: 1, name: 'Другое имя', description: null })
    expect(dishPatch({ ...original, recipe: [...original.recipe, ...values.recipe] }, baseline)).toEqual({ expected_revision: 1, recipe: [...baseline.fields.recipe, { product_id: id, quantity_per_portion: '0.001' }] })
    expect(sameRecipe(baseline.fields.recipe, [...baseline.fields.recipe].reverse())).toBe(true)
    expect(updateDishSchema.safeParse(dishPatch(original, baseline)).success).toBe(false)
    expect(updateDishSchema.safeParse({ expected_revision: 2147483647, name: 'x' }).success).toBe(false)
  })
  it('isolates draft fields and maps only known server paths', () => {
    expect(dishDraftSchema.safeParse({ resourceId: 'new', values, baseline: null, uncertain: true, review: false }).success).toBe(true)
    expect(dishFieldPath('body.recipe[0].quantity_per_portion')).toBe('recipe.0.quantity_per_portion')
    expect(dishFieldPath('/body/recipe/2/product_id')).toBe('recipe.2.product_id')
    expect(dishFieldPath('password')).toBeUndefined()
  })
})
