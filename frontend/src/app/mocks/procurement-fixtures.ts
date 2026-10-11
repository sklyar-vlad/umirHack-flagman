import type { ProcurementItem } from '@/entities/procurement'
import { parseQuantity } from '@/shared/lib/quantity'
import { inventoryItems } from './inventory-fixtures'
import { consumedCompositionAmounts } from './order-composition-fixtures'
import { receivedSupplies } from './reception-fixtures'
import { stopListDishes } from './stop-list-fixtures'
import { getMockScenario } from './scenarios'
const decimal = (value: bigint) => `${value / 1000n}.${String(value % 1000n).padStart(3, '0')}`
/** Synthetic server computation only; no business arithmetic is used by the UI. */
export function procurementItems(): ProcurementItem[] {
  if (getMockScenario() === 'procurement-empty') return []
  const now = new Date().toISOString(), dishes = stopListDishes()
  const consumed = Object.values(consumedCompositionAmounts()).reduce((sum, raw) => sum + parseQuantity(String(raw))!, 0n)
  const candidates = inventoryItems.map((item, index) => {
    // Dedicated demo targets exercise pagination and an inactive product with a goal.
    const target = index === 1 ? '3000.000' : index >= 5 ? '1000.001' : item.target_stock
    if (target === null || parseQuantity(target) === 0n) return null
    const receipts = receivedSupplies().flatMap((supply) => supply.items).filter((line) => line.product.id === item.product.id && line.batch.expires_at > now).reduce((sum, line) => sum + parseQuantity(line.batch.remaining_quantity)!, 0n)
    const base = getMockScenario() === 'procurement-expired' && index === 0 ? 0n : parseQuantity(item.available_quantity)!
    const available = base + receipts - (index === 0 ? consumed : 0n)
    const stock = available > 0n ? available : 0n, required = parseQuantity(target)! - stock
    if (required <= 0n) return null
    const affected = dishes.filter((dish) => dish.shortages.some((shortage) => shortage.product_id === item.product.id)).map(({ id, name }) => ({ id, name }))
    const reason_codes: ProcurementItem['reason_codes'] = ['below_target']
    if (stock === 0n) { reason_codes.push('no_valid_stock'); if (parseQuantity(item.expired_quantity)! > 0n) reason_codes.push('only_expired_stock') }
    return { product_id: item.product.id, product_name: item.product.name, unit: item.product.unit, display_unit: item.product.display_unit,
      available_quantity: decimal(stock), expired_quantity: item.expired_quantity, target_stock: target, recommended_quantity: decimal(required), reason_codes,
      reason: `Годный остаток ${decimal(stock)} ${item.product.unit}; целевой запас ${target} ${item.product.unit}. Просрочка не уменьшает закупку.`, affected_dishes: affected } satisfies ProcurementItem
  }).filter((item): item is ProcurementItem => item !== null)
  return candidates.sort((a, b) => b.affected_dishes.length - a.affected_dishes.length || Number(/^0+(?:\.0+)?$/.test(b.available_quantity)) - Number(/^0+(?:\.0+)?$/.test(a.available_quantity)) || a.product_name.localeCompare(b.product_name, 'ru') || a.product_id.localeCompare(b.product_id))
}
