import type { Dashboard } from '@/entities/dashboard'
import type { Role } from '@/entities/session'
import { parseQuantity, serializeQuantity } from '@/shared/lib/quantity'
import { inventoryBatches, inventoryItems, mockId } from './inventory-fixtures'
import { consumedCompositionAmounts } from './order-composition-fixtures'
import { receivedSupplies } from './reception-fixtures'
import { hotOfferItems } from './hot-offer-fixtures'
import { procurementItems } from './procurement-fixtures'
import { stopListDishes } from './stop-list-fixtures'
import { getMockScenario } from './scenarios'

/** Synthetic snapshot only; production never computes dashboard counters in the browser. */
export function dashboardSnapshot(role: Role = 'manager'): Dashboard {
  const scenario = getMockScenario(), now = Date.now(), server_time = new Date(now).toISOString()
  const settings = { restaurant_name: 'Тестовый ресторан', timezone: 'Europe/Moscow', expiring_threshold_hours: 24 }
  if (scenario === 'dashboard-empty') return { expiring_batches_count: 0, expired_batches_count: 0, stop_list_dishes_count: 0, out_of_stock_products_count: 0, attention_batches: [], attention_total: 0, hot_offers: [], procurement_recommendations: [], settings, meta: { server_time } }
  const consumed = consumedCompositionAmounts()
  // Base fixtures use a deliberately fixed snapshot. Move their relative deadlines
  // to this single synthetic server_time without mixing browser-derived UI counts.
  const base = inventoryBatches.map((batch) => ({ ...batch, expires_at: new Date(now + (scenario === 'dashboard-expired' && batch.id === mockId(202) ? -1 : batch.remaining_seconds) * 1000).toISOString() }))
  const receipts = receivedSupplies().flatMap((supply) => supply.items.map((line) => line.batch))
  const batches = [...base, ...receipts].map((batch) => ({ ...batch, remaining_quantity: serializeQuantity(parseQuantity(batch.remaining_quantity)! - parseQuantity(String(consumed[batch.id] ?? 0))!)! }))
    .filter((batch) => parseQuantity(batch.remaining_quantity)! > 0n)
  const attention: Dashboard['attention_batches'] = batches.filter((batch) => Date.parse(batch.expires_at) <= now + 86400000)
    .sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id)).map((batch) => ({
      batch_id: batch.id, product_id: batch.product_id, product_name: inventoryItems.find((item) => item.product.id === batch.product_id)!.product.name,
      supply_id: batch.supply.id, supply_number: batch.supply.number, line_number: batch.line_number,
      remaining_quantity: batch.remaining_quantity, unit: batch.unit, status: Date.parse(batch.expires_at) <= now ? 'expired' : 'expiring',
      expires_at: batch.expires_at, remaining_seconds: Math.floor((Date.parse(batch.expires_at) - now) / 1000),
    }))
  if (scenario === 'dashboard-preview') attention.push(...Array.from({ length: 22 }, (_, i) => ({ ...attention[1], batch_id: mockId(950 + i), line_number: i + 30 })))
  attention.sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.batch_id.localeCompare(b.batch_id))
  const expiring = attention.filter((batch) => batch.status === 'expiring').length, expired = attention.length - expiring
  const missing = inventoryItems.filter((item) => !batches.some((batch) => batch.product_id === item.product.id && Date.parse(batch.expires_at) > now)).length
  return { expiring_batches_count: expiring, expired_batches_count: expired, stop_list_dishes_count: stopListDishes().length, out_of_stock_products_count: missing,
    attention_total: attention.length, attention_batches: attention.slice(0, 20),
    hot_offers: role === 'storekeeper' ? [] : hotOfferItems().slice(0, 3), procurement_recommendations: role === 'order_operator' ? [] : procurementItems().slice(0, 3), settings, meta: { server_time } }
}
