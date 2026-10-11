import type { SupplyDetail, SupplySummary } from '@/entities/supply'
import { inventoryBatches, inventoryItems, inventoryServerTime, mockId } from './inventory-fixtures'

/** One synthetic snapshot shared with the warehouse; repeated products keep every batch. */
export function supplyDetailFixture(summary: SupplySummary): SupplyDetail {
  const batches = summary.id === mockId(100) ? inventoryBatches : Array.from({ length: summary.items_count }, (_, index) => ({
    ...inventoryBatches[0], id: mockId(10000 + index), line_number: index + 1,
    supply: { id: summary.id, number: summary.number, received_at: summary.received_at },
  }))
  return {
    ...summary, meta: { server_time: inventoryServerTime },
    items: batches.map((batch) => {
      const { id, name, unit, display_unit } = inventoryItems.find((item) => item.product.id === batch.product_id)!.product
      return { product: { id, name, unit, display_unit }, batch }
    }),
  }
}
