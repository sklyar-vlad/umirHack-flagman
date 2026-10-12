import type { Dish, DishPreview } from '@/entities/dish'
import type { PreviewDish } from '@/shared/api/dish-work'
import { catalogInventoryItems, catalogProducts } from './catalog-fixtures'

let saved: Dish[] = []
const key = 'flagman.mock.dishes.v1'
function read() { try { saved = JSON.parse(sessionStorage.getItem(key) ?? JSON.stringify(saved)) as Dish[] } catch { /* Node tests use memory. */ } return saved }
export function saveEditorDish(dish: Dish) { saved = [...read().filter((item) => item.id !== dish.id), dish]; try { sessionStorage.setItem(key, JSON.stringify(saved)) } catch { /* Node tests. */ } }
export function resetDishEditorFixtures() { saved = []; try { sessionStorage.removeItem(key) } catch { /* Node tests. */ } }
export function applyEditorDishes(dishes: Dish[]) {
  const changed = read()
  return [...dishes.map((dish) => changed.find((item) => item.id === dish.id) ?? dish), ...changed.filter((dish) => !dishes.some((item) => item.id === dish.id))]
}
const amount = (raw: string) => { const [whole, fraction = ''] = raw.split('.'); return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0')) }
const decimal = (value: bigint) => `${value / 1000n}.${String(value % 1000n).padStart(3, '0')}`
/** Synthetic server calculation only; quantities use exact thousandths including large aggregates. */
export function editorPreview(recipe: PreviewDish['recipe'], stock = catalogInventoryItems()): DishPreview {
  let portions: bigint | undefined
  const requirements = recipe.map((line) => {
    const product = catalogProducts().find((product) => product.id.toLowerCase() === line.product_id.toLowerCase())!
    const required = amount(line.quantity_per_portion), available = amount(stock.find((item) => item.product.id === product.id)?.available_quantity ?? '0')
    const candidate = available / required
    portions = portions === undefined || candidate < portions ? candidate : portions
    return { product_id: product.id, product_name: product.name, unit: product.unit, required_quantity: line.quantity_per_portion, available_quantity: decimal(available), missing_quantity: decimal(required > available ? required - available : 0n) }
  })
  const shortages = requirements.filter((line) => amount(line.missing_quantity) > 0n).map((line) => {
    const expired = stock.find((item) => item.product.id === line.product_id)?.expired_quantity ?? '0.000'
    return { ...line, expired_quantity: expired, reason_code: amount(line.available_quantity) === 0n && amount(expired) > 0n ? 'only_expired_stock' as const : 'ingredient_shortage' as const }
  })
  return { is_available: !shortages.length, available_portions: String(portions ?? 0n), requirements, shortages, meta: { server_time: new Date().toISOString() } }
}
export function withEditorAvailability(dish: Dish): Dish {
  if (!dish.recipe.length) return { ...dish, is_available: false, available_portions: '0', shortages: [], unavailability_reasons: dish.is_active ? ['recipe_missing'] : ['dish_inactive'] }
  const preview = editorPreview(dish.recipe)
  return { ...dish, shortages: preview.shortages, meta: preview.meta, is_available: dish.is_active && preview.is_available, available_portions: dish.is_active ? preview.available_portions : '0', unavailability_reasons: !dish.is_active ? ['dish_inactive'] : preview.shortages.map((line) => line.reason_code) }
}
