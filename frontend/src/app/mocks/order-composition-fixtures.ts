import { applyBatchWriteOff, removedWriteOffQuantity } from './write-off-fixtures'
import { catalogInventoryItems } from './catalog-fixtures'
import { parseQuantity, serializeQuantity } from '@/shared/lib/quantity'
import { applyEditorDishes } from './dish-editor-fixtures'
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
  return applyEditorDishes(['Суп дня', 'Салат дня', 'Отключённое блюдо', 'Без рецептуры'].map((name, i) => ({
    id: mockId(800 + i), name, description: i === 0 ? 'Горячий суп из общего запаса продуктов' : null,
    is_active: i !== 2 && !(inactive && i === 0), revision: (changed || inactive) && i === 0 ? 2 : 1, recipe_version: i === 3 ? null : changed && i === 0 ? 2 : 1,
    recipe: i === 3 ? [] : [{ ...ingredient, quantity_per_portion: changed && i === 0 ? '150.000' : '100.000' }],
    is_available: i < 2 && !(inactive && i === 0) && availableStock() >= (changed && i === 0 ? 150 : 100), available_portions: i < 2 ? String(Math.floor(availableStock() / (changed && i === 0 ? 150 : 100))) : '0', shortages: [],
    unavailability_reasons: i === 2 || inactive && i === 0 ? ['dish_inactive'] : i === 3 ? ['recipe_missing'] : [], meta: meta(),
  })))
}
export function compositionCheck(body: OrderRequest, changed = false, inactive = false, shortage = false): OrderCheck {
  changed ||= currentRecipeChanged; inactive ||= currentInactive
  const dishes = compositionDishes(changed, inactive), issues: OrderCheck['issues'] = []
  const required = new Map<string, { ingredient: Dish['recipe'][number]; amount: bigint }>()
  const items = body.items.map((line) => {
    const dish = dishes.find((item) => item.id === line.dish_id)!
    if (!dish.is_active) issues.push({ dish_id: dish.id, code: 'dish_inactive', message: `${dish.name}: блюдо отключено`, current_recipe_version: dish.recipe_version })
    else if (!dish.recipe_version) issues.push({ dish_id: dish.id, code: 'recipe_missing', message: 'Рецептура отсутствует', current_recipe_version: null })
    else if (line.recipe_version != null && line.recipe_version !== dish.recipe_version) issues.push({ dish_id: dish.id, code: 'recipe_changed', message: `${dish.name}: рецептура изменилась`, current_recipe_version: dish.recipe_version })
    if (dish.is_active && dish.recipe_version) for (const ingredient of dish.recipe) {
      const prior = required.get(ingredient.product_id)
      required.set(ingredient.product_id, { ingredient, amount: (prior?.amount ?? 0n) + parseQuantity(ingredient.quantity_per_portion)! * BigInt(line.portions) })
    }
    return { dish_id: dish.id, dish_name: dish.name, portions: line.portions, recipe_version: dish.recipe_version }
  })
  const stocks = catalogInventoryItems()
  const aggregate = (raw: string) => { const [whole, fraction = ''] = raw.split('.'); return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0')) }
  const decimal = (value: bigint) => `${value / 1000n}.${String(value % 1000n).padStart(3, '0')}`
  const requirements = [...required.values()].map(({ ingredient, amount }) => {
    const stock = stocks.find((item) => item.product.id === ingredient.product_id)
    const consumed = composedOrders().flatMap((order) => order.items.flatMap((item) => item.allocations)).filter((line) => line.product_id === ingredient.product_id).reduce((sum, line) => sum + parseQuantity(line.quantity)!, 0n)
    const remaining = aggregate(stock?.available_quantity ?? '0') - consumed - removedWriteOffQuantity(ingredient.product_id, false)
    const available = shortage || remaining < 0n ? 0n : remaining
    return { product_id: ingredient.product_id, product_name: ingredient.product_name, unit: ingredient.unit, required_quantity: decimal(amount), available_quantity: decimal(available), missing_quantity: decimal(amount > available ? amount - available : 0n) }
  })
  const shortages = requirements.filter((line) => aggregate(line.missing_quantity) > 0n).map((line) => {
    const expired = line.product_id === product.id ? '500.000' : stocks.find((item) => item.product.id === line.product_id)?.expired_quantity ?? '0.000'
    return { ...line, expired_quantity: expired, reason_code: aggregate(line.available_quantity) === 0n && aggregate(expired) > 0n ? 'only_expired_stock' as const : 'ingredient_shortage' as const }
  })
  return { can_create: !issues.length && !shortages.length, items, requirements, shortages, issues, meta: meta() }
}

interface SavedOrder { key: string; body: string; order?: OrderDetail; refusal?: OrderRefusal; reason?: OrderReason; attempt?: OrderAttempt }
let saved: SavedOrder[] = []
const storageKey = 'flagman.mock.orders.v1'
function read() { try { saved = JSON.parse(sessionStorage.getItem(storageKey) ?? '[]') as SavedOrder[] } catch {  } return saved }
function save(value: SavedOrder) { saved.push(value); try { sessionStorage.setItem(storageKey, JSON.stringify(saved)) } catch {  } }
export function resetCompositionFixtures() { saved = []; currentRecipeChanged = false; currentInactive = false; try { sessionStorage.removeItem(storageKey) } catch {  } }
export const savedOrder = (key: string) => read().find((value) => value.key === key)
export const composedOrders = () => read().flatMap((value) => value.order ? [{ ...value.order, total_portions: value.order.items.reduce((sum, item) => sum + item.portions, 0) }] : [])
export const composedAttempts = () => read().flatMap((value) => value.attempt ? [value.attempt] : [])
export function acceptOrder(key: string, body: OrderRequest, check: OrderCheck): OrderDetail {
  const consumed = new Map<string, bigint>()
  for (const order of composedOrders()) for (const item of order.items) for (const line of item.allocations) consumed.set(line.batch_id, (consumed.get(line.batch_id) ?? 0n) + parseQuantity(line.quantity)!)
  const balances = inventoryBatches.map(applyBatchWriteOff).filter((batch) => ['valid', 'expiring'].includes(batch.status)).sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id)).map((batch) => ({ ...batch, remaining: parseQuantity(batch.remaining_quantity)! - (consumed.get(batch.id) ?? 0n) }))
  const order: OrderDetail = { id: crypto.randomUUID(), number: String(1001 + composedOrders().length), status: 'completed', created_at: new Date().toISOString(), created_by: { id: mockId(900), name: 'Тестовый оператор' },
    items: check.items.map((item) => {
      const recipe = compositionDishes(item.recipe_version === 2).find((dish) => dish.id === item.dish_id)!.recipe
      const allocations: OrderDetail['items'][number]['allocations'] = []
      for (const ingredient of recipe) {
        let needed = BigInt(item.portions) * parseQuantity(ingredient.quantity_per_portion)!
        for (const batch of balances.filter((batch) => batch.product_id === ingredient.product_id)) {
          if (!needed) break
          const quantity = needed < batch.remaining ? needed : batch.remaining
          if (quantity <= 0n) continue
          batch.remaining -= quantity; needed -= quantity
          allocations.push({ movement_id: crypto.randomUUID(), product_id: ingredient.product_id, product_name: ingredient.product_name, unit: ingredient.unit, batch_id: batch.id,
            supply_id: batch.supply.id, supply_number: batch.supply.number, line_number: batch.line_number,
            quantity: serializeQuantity(quantity)!, batch_remaining_quantity_after: serializeQuantity(batch.remaining)! })
        }
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
  return inventoryBatches.map(applyBatchWriteOff).filter((batch) => batch.product_id === product.id && ['valid', 'expiring'].includes(batch.status))
    .sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id))
    .map((batch) => ({ ...batch, remaining: Number(batch.remaining_quantity) - (consumed[batch.id] ?? 0) }))
}
function availableStock() { return remainingCompositionBatches().reduce((sum, batch) => sum + batch.remaining, 0) }
