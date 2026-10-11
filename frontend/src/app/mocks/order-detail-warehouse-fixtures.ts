import type { Batch, InventoryItem } from '@/entities/inventory'
import type { SupplyDetail } from '@/entities/supply'
import { serializeQuantity, parseQuantity } from '@/shared/lib/quantity'
import { inventoryCategories, inventoryServerTime } from './inventory-fixtures'
import { orderDetailFixture, splitOrderDetailFixture } from './order-detail-fixtures'
import { getMockScenario } from './scenarios'

/** Explicit demo-only warehouse snapshot, not a reconstruction of real current stock. */
export function orderDemoSupplies(): SupplyDetail[] {
  const scenario = getMockScenario()
  if (scenario !== 'order-detail-demo' && scenario !== 'order-detail-split') return []
  const order = scenario === 'order-detail-split' ? splitOrderDetailFixture : orderDetailFixture
  const supplies = new Map<string, SupplyDetail>()
  const receivedAt = '2026-10-08T09:00:00Z'
  for (const row of order.items.flatMap((item) => item.allocations)) {
    let supply = supplies.get(row.supply_id)
    if (!supply) {
      supply = { id: row.supply_id, number: row.supply_number, status: 'received', created_at: receivedAt, received_at: receivedAt, created_by: order.created_by, items_count: 0, items: [], meta: { server_time: inventoryServerTime } }
      supplies.set(row.supply_id, supply)
    }
    const batch: Batch = {
      id: row.batch_id, product_id: row.product_id, unit: row.unit,
      supply: { id: supply.id, number: supply.number, received_at: receivedAt }, line_number: row.line_number,
      initial_quantity: serializeQuantity(parseQuantity(row.quantity)! + parseQuantity(row.batch_remaining_quantity_after)!)!, remaining_quantity: row.batch_remaining_quantity_after,
      produced_at: null, shelf_life_hours: null, expiry_source: 'explicit', expires_at: '2026-10-14T12:00:00Z',
      remaining_seconds: 259200, status: parseQuantity(row.batch_remaining_quantity_after) === 0n ? 'depleted' : 'valid', created_at: receivedAt,
    }
    supply.items.push({ product: { id: row.product_id, name: row.product_name, unit: row.unit, display_unit: row.unit }, batch })
    supply.items_count = supply.items.length
  }
  return [...supplies.values()]
}

export function orderDemoInventory(): InventoryItem[] {
  const items = new Map<string, InventoryItem>()
  for (const { product, batch } of orderDemoSupplies().flatMap((supply) => supply.items)) {
    let item = items.get(product.id)
    if (!item) {
      item = {
        product: { ...product, is_active: true, category: inventoryCategories[1] },
        physical_quantity: '0.000', available_quantity: '0.000', expiring_quantity: '0.000', expired_quantity: '0.000',
        target_stock: null, deficit_quantity: null, batches_count: 0, nearest_expires_at: null, remaining_seconds: null,
        has_expiring: false, has_expired: false, is_out_of_stock: true, status: 'out_of_stock',
      }
      items.set(product.id, item)
    }
    item.physical_quantity = serializeQuantity(parseQuantity(item.physical_quantity)! + parseQuantity(batch.remaining_quantity)!)!
    item.available_quantity = item.physical_quantity
    if (batch.status !== 'depleted') {
      item.batches_count += 1; item.nearest_expires_at = batch.expires_at; item.remaining_seconds = batch.remaining_seconds
      item.is_out_of_stock = false; item.status = 'normal'
    }
  }
  return [...items.values()]
}
