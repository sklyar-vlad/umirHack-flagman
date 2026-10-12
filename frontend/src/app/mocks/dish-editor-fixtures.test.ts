import { beforeEach, describe, expect, it } from 'vitest'
import { dishPreviewSchema } from '@/entities/dish'
import { orderDetailSchema } from '@/entities/order'
import { resetCatalogFixtures } from './catalog-fixtures'
import { mockId } from './inventory-fixtures'
import { compositionDishes, compositionCheck, acceptOrder, savedOrder, resetCompositionFixtures } from './order-composition-fixtures'
import { editorPreview, resetDishEditorFixtures, saveEditorDish, withEditorAvailability } from './dish-editor-fixtures'
beforeEach(() => { resetCatalogFixtures(); resetDishEditorFixtures(); resetCompositionFixtures() })
describe('F20 explicit synthetic recipe snapshots', () => {
  it('uses exact large quantities and minimum portions across recipe products', () => {
    const preview = editorPreview([{ product_id: mockId(13), quantity_per_portion: '99999999999.999' }, { product_id: mockId(14), quantity_per_portion: '2.000' }])
    expect(dishPreviewSchema.safeParse(preview).success).toBe(true)
    expect(preview.available_portions).toBe('2'); expect(preview.requirements[0].available_quantity).toBe('199999999999.998')
  })
  it('changed recipe rechecks all ingredients and keeps earlier saved allocations immutable', () => {
    const dish = compositionDishes()[0], body = { items: [{ dish_id: dish.id, recipe_version: 1, portions: 1 }] }
    const oldOrder = acceptOrder('old-order', body, compositionCheck(body)), snapshot = structuredClone(oldOrder)
    const sugar = { product_id: mockId(12), product_name: 'Сахар', unit: 'g' as const, quantity_per_portion: '0.001' }
    saveEditorDish(withEditorAvailability({ ...dish, recipe_version: 2, revision: 2, recipe: [...dish.recipe, sugar] }))
    const checked = compositionCheck(body)
    expect(checked.can_create).toBe(false); expect(checked.issues).toMatchObject([{ code: 'recipe_changed', current_recipe_version: 2 }]); expect(checked.shortages).toMatchObject([{ product_id: sugar.product_id, missing_quantity: '0.001' }])
    expect(savedOrder('old-order')?.order).toEqual(snapshot)
  })
  it('new multi-product recipe preserves separate exact allocation units in created mock order', () => {
    const dish = compositionDishes()[0]
    saveEditorDish(withEditorAvailability({ ...dish, recipe_version: 2, revision: 2, recipe: [...dish.recipe, { product_id: mockId(14), product_name: 'Яйцо', unit: 'pcs', quantity_per_portion: '2.000' }] }))
    const body = { items: [{ dish_id: dish.id, recipe_version: 2, portions: 1 }] }, checked = compositionCheck(body)
    expect(checked.can_create).toBe(true); expect(checked.requirements.map((line) => line.required_quantity)).toEqual(['100.000', '2.000'])
    const order = acceptOrder('multi-product', body, checked)
    expect(orderDetailSchema.safeParse(order).success).toBe(true)
    expect(order.items[0].allocations.map((line) => [line.unit, line.quantity])).toEqual([['ml', '100.000'], ['pcs', '2.000']])
  })
})
