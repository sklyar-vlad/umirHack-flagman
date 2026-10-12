import { applyEditorDishes } from './dish-editor-fixtures'
import type { Dish } from '@/entities/dish'
import { compositionDishes } from './order-composition-fixtures'
import { mockId } from './inventory-fixtures'
/** Synthetic F19 records are isolated from the existing order basket fixtures. */
export function dishCatalog(newVersion = false): Dish[] {
  const dishes = compositionDishes(), soup = dishes[0]
  const extra: Dish[] = Array.from({ length: 21 }, (_, index) => ({ ...soup, id: mockId(850 + index), name: `Тестовое блюдо ${String(index + 1).padStart(2, '0')}`, description: null }))
  const snapshot = [...dishes, ...extra]
  if (newVersion) snapshot[0] = { ...soup, revision: soup.revision + 1, recipe_version: (soup.recipe_version ?? 0) + 1, recipe: [...soup.recipe, { product_id: mockId(12), product_name: 'Сахар', unit: 'g', quantity_per_portion: '0.001' }], available_portions: '7' }
  return applyEditorDishes(snapshot)
}
export function filterDishCatalog(dishes: Dish[], search: string, active: string, availability: string) {
  return dishes.filter((dish) => dish.name.toLocaleLowerCase('ru').includes(search.toLocaleLowerCase('ru')) && (active === 'all' || dish.is_active === (active === 'true')) && (availability === 'all' || dish.is_available === (availability === 'available')))
    .sort((a, b) => a.name.localeCompare(b.name, 'ru') || a.id.localeCompare(b.id))
}
