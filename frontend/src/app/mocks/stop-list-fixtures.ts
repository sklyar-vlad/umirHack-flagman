import { removedWriteOffQuantity } from './write-off-fixtures'
import { applyEditorDishes } from './dish-editor-fixtures'
import type { Dish } from '@/entities/dish'
import { parseQuantity } from '@/shared/lib/quantity'
import { compositionDishes, consumedCompositionAmounts } from './order-composition-fixtures'
import { inventoryItems, mockId } from './inventory-fixtures'
import { receivedSupplies } from './reception-fixtures'
import { getMockScenario } from './scenarios'

const decimal = (value: bigint) => `${value / 1000n}.${String(value % 1000n).padStart(3, '0')}`
/** Synthetic per-portion snapshots, shared with F08/F10 saved mock mutations. */
export function stopListDishes(): Dish[] {
  const scenario = getMockScenario(), now = new Date().toISOString()
  if (scenario === 'stop-list-empty') return []
  const milk = inventoryItems[0].product, sugar = inventoryItems[2].product
  const consumed = Object.values(consumedCompositionAmounts()).reduce((sum, amount) => sum + parseQuantity(String(amount))!, 0n)
  const received = receivedSupplies().flatMap((supply) => supply.items).filter((line) => line.product.id === milk.id && line.batch.expires_at > now).reduce((sum, line) => sum + parseQuantity(line.batch.remaining_quantity)!, 0n)
  const available = scenario === 'stop-list-expired' ? 0n : 1_500_000n + received - consumed - removedWriteOffQuantity(milk.id, false)
  const required = scenario === 'stop-list-recipe' ? 100_000n : 1_500_001n
  const make = (id: number, name: string, product = milk, amount = required, stock = available, expired = 200_000n - removedWriteOffQuantity(milk.id, true)): Dish => {
    const missing = amount > stock ? amount - stock : 0n
    const reason = stock === 0n && expired > 0n ? 'only_expired_stock' : 'ingredient_shortage'
    return { id: mockId(id), name, description: 'Тестовый снимок потребности на одну порцию', is_active: true, revision: 1, recipe_version: scenario === 'stop-list-recipe' ? 2 : 1,
      recipe: [{ product_id: product.id, product_name: product.name, unit: product.unit, quantity_per_portion: decimal(amount) }], is_available: missing === 0n, available_portions: String(stock / amount),
      shortages: missing > 0n ? [{ product_id: product.id, product_name: product.name, unit: product.unit, required_quantity: decimal(amount), available_quantity: decimal(stock), missing_quantity: decimal(missing), expired_quantity: decimal(expired), reason_code: reason }] : [],
      unavailability_reasons: missing > 0n ? [reason] : [], meta: { server_time: now } }
  }
  const missingRecipe = compositionDishes().find((dish) => dish.recipe_version === null)!
  const dishes = [make(850, 'Молочный суп'), make(851, 'Крем — только просрочка', milk, 240_000n, 0n, 300_000n), make(852, 'Десерт без остатка', sugar, 20_001n, 0n, 0n),
    ...Array.from({ length: 23 }, (_, i) => make(860 + i, `Тестовое блюдо ${String(i + 1).padStart(2, '0')}`, sugar, 10_000n, 0n, 0n)),
    { ...missingRecipe, meta: { server_time: now } },
    ...compositionDishes().filter((dish) => dish.recipe_version !== null).map((dish, index) => ({
      ...make(800 + index, dish.name, milk, parseQuantity(dish.recipe[0].quantity_per_portion)!, available),
      id: dish.id, is_active: dish.is_active, recipe_version: dish.recipe_version,
    })),
  ]
  return applyEditorDishes(dishes).filter((dish) => dish.is_active && !dish.is_available).sort((a, b) => a.name.localeCompare(b.name, 'ru') || a.id.localeCompare(b.id))
}
