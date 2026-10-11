import type { Dish } from '@/entities/dish'
import type { OrderAttempt, OrderCheck, OrderDetail, OrderReason, OrderRefusal } from '@/entities/order'
import type { OrderRequest } from '@/shared/api/order-work'
import { inventoryBatches, inventoryItems, mockId } from './inventory-fixtures'

const product = inventoryItems[0].product
const ingredient = { product_id: product.id, product_name: product.name, unit: product.unit, quantity_per_portion: '100.000' }
let currentRecipeChanged = false, currentInactive = false
export function changeCompositionRecipe(inactive = false) { if (inactive) currentInactive = true; else currentRecipeChanged = true }
const meta = () => ({ server_time: new Date().toISOString() })
export function compositionDishes(changed = false, inactive = false): Dish[] {
  changed ||= currentRecipeChanged; inactive ||= currentInactive
  return ['Суп дня', 'Салат дня', 'Отключённое блюдо', 'Без рецептуры'].map((name, i) => ({
    id: mockId(800 + i), name, description: i === 0 ? 'Горячий суп из общего запаса продуктов' : null,
    is_active: i !== 2 && !(inactive && i === 0), revision: (changed || inactive) && i === 0 ? 2 : 1, recipe_version: i === 3 ? null : changed && i === 0 ? 2 : 1,
    recipe: i === 3 ? [] : [{ ...ingredient, quantity_per_portion: changed && i === 0 ? '150.000' : '100.000' }],
    is_available: i < 2 && !(inactive && i === 0) && availableStock() >= (changed && i === 0 ? 150 : 100), available_portions: i < 2 ? String(Math.floor(availableStock() / (changed && i === 0 ? 150 : 100))) : '0', shortages: [],
    unavailability_reasons: i === 2 || inactive && i === 0 ? ['dish_inactive'] : i === 3 ? ['recipe_missing'] : [], meta: meta(),
  }))
}
export function compositionCheck(body: OrderRequest, changed = false, inactive = false, shortage = false): OrderCheck {
  changed ||= currentRecipeChanged; inactive ||= currentInactive
  const dishes = compositionDishes(changed, inactive), issues: OrderCheck['issues'] = []
  let total = 0
  const items = body.items.map((line) => {
    const dish = dishes.find((item) => item.id === line.dish_id)!
    if (!dish.is_active) issues.push({ dish_id: dish.id, code: 'dish_inactive', message: `${dish.name}: блюдо отключено`, current_recipe_version: dish.recipe_version })
    else if (!dish.recipe_version) issues.push({ dish_id: dish.id, code: 'recipe_missing', message: 'Рецептура отсутствует', current_recipe_version: null })
    else if (line.recipe_version != null && line.recipe_version !== dish.recipe_version) issues.push({ dish_id: dish.id, code: 'recipe_changed', message: `${dish.name}: рецептура изменилась`, current_recipe_version: dish.recipe_version })
    if (dish.is_active && dish.recipe_version) total += line.portions * (changed && dish.id === mockId(800) ? 150 : 100)
    return { dish_id: dish.id, dish_name: dish.name, portions: line.portions, recipe_version: dish.recipe_version }
  })
  const available = shortage ? 0 : availableStock()
  const requirement = { product_id: product.id, product_name: product.name, unit: product.unit, required_quantity: `${total}.000`, available_quantity: `${available}.000`, missing_quantity: `${Math.max(0, total - available)}.000` }
  return { can_create: !issues.length && total <= available, items, requirements: [requirement], shortages: total > available ? [{ ...requirement, expired_quantity: '500.000', reason_code: shortage ? 'only_expired_stock' : 'ingredient_shortage' }] : [], issues, meta: meta() }
}
interface SavedOrder { key: string; body: string; order?: OrderDetail; refusal?: OrderRefusal; reason?: OrderReason; attempt?: OrderAttempt }
let saved: SavedOrder[] = []
const storageKey = 'flagman.mock.orders.v1'
function read() { try { saved = JSON.parse(sessionStorage.getItem(storageKey) ?? '[]') as SavedOrder[] } catch { /* Node tests. */ } return saved }
function save(value: SavedOrder) { saved.push(value); try { sessionStorage.setItem(storageKey, JSON.stringify(saved)) } catch { /* Node tests. */ } }
export function resetCompositionFixtures() { saved = []; currentRecipeChanged = false; currentInactive = false; try { sessionStorage.removeItem(storageKey) } catch { /* Node tests. */ } }
export const savedOrder = (key: string) => read().find((value) => value.key === key)
export const composedOrders = () => read().flatMap((value) => value.order ? [{ ...value.order, total_portions: value.order.items.reduce((sum, item) => sum + item.portions, 0) }] : [])
export const composedAttempts = () => read().flatMap((value) => value.attempt ? [value.attempt] : [])
export function acceptOrder(key: string, body: OrderRequest, check: OrderCheck): OrderDetail {
  const balances = remainingCompositionBatches()
  const order: OrderDetail = { id: crypto.randomUUID(), number: String(1001 + composedOrders().length), status: 'completed', created_at: new Date().toISOString(), created_by: { id: mockId(900), name: 'Тестовый оператор' },
    items: check.items.map((item) => {
      let needed = item.portions * (item.recipe_version === 2 ? 150 : 100)
      const allocations: OrderDetail['items'][number]['allocations'] = []
      for (const batch of balances) {
        if (!needed) break
        const quantity = Math.min(needed, batch.remaining)
        if (!quantity) continue
        batch.remaining -= quantity; needed -= quantity
        allocations.push({ movement_id: crypto.randomUUID(), product_id: product.id, product_name: product.name, unit: product.unit, batch_id: batch.id,
          supply_id: batch.supply.id, supply_number: batch.supply.number, line_number: batch.line_number,
          quantity: `${quantity}.000`, batch_remaining_quantity_after: `${batch.remaining}.000` })
      }
      return { ...item, recipe_version: item.recipe_version!, id: crypto.randomUUID(), allocations }
    }),
  }
  save({ key, body: JSON.stringify(body), order }); return order
}
export function rejectOrder(key: string, body: OrderRequest, check: OrderCheck, reason: OrderReason): OrderRefusal {
  const refusal = { attempt_id: crypto.randomUUID(), requirements: check.requirements, shortages: check.shortages, issues: check.issues }
  const attempt: OrderAttempt = { id: refusal.attempt_id, created_at: new Date().toISOString(), created_by: { id: mockId(900), name: 'Тестовый оператор' }, reason_code: reason, items: check.items, shortages: check.shortages, issues: check.issues }
  save({ key, body: JSON.stringify(body), refusal, reason, attempt }); return refusal
}

export function consumedCompositionAmounts() {
  const amounts: Record<string, number> = {}
  for (const order of composedOrders()) for (const item of order.items) for (const allocation of item.allocations) amounts[allocation.batch_id] = (amounts[allocation.batch_id] ?? 0) + Number(allocation.quantity)
  return amounts
}
function remainingCompositionBatches() {
  const consumed = consumedCompositionAmounts()
  return inventoryBatches.filter((batch) => batch.product_id === product.id && ['valid', 'expiring'].includes(batch.status))
    .sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id))
    .map((batch) => ({ ...batch, remaining: Number(batch.remaining_quantity) - (consumed[batch.id] ?? 0) }))
}
function availableStock() { return remainingCompositionBatches().reduce((sum, batch) => sum + batch.remaining, 0) }
