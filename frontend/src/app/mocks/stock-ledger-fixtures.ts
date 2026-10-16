import type { StockMovement } from '@/entities/stock-movement'
import { inventoryBatches, mockId } from './inventory-fixtures'
import { orderDetailFixture } from './order-detail-fixtures'

export const mockMovements: StockMovement[] = Array.from({ length: 65 }, (_, index) => {
  const type = (['receipt', 'order_consumption', 'manual_writeoff', 'expiration'] as const)[index % 4]
  const batch = inventoryBatches[type === 'receipt' ? index / 4 : index % 3]
  return { id: mockId(2000 + index), product_id: batch.product_id, batch_id: batch.id,
    order_id: type === 'order_consumption' ? orderDetailFixture.id : null, order_item_id: type === 'order_consumption' ? orderDetailFixture.items[0].id : null,
    type, delta_quantity: type === 'receipt' ? '1000.000' : type === 'order_consumption' ? '-12.125' : '-100.000',
    remaining_quantity_after: type === 'receipt' ? '1000.000' : type === 'order_consumption' ? '987.875' : '0.000',
    product_name: 'Молоко · название при операции', unit: 'ml', actor: type === 'expiration' ? null : { id: mockId(800), name: 'Анна Смирнова' },
    occurred_at: new Date(Date.UTC(2026, 9, 10, 9) - index * 60000).toISOString(), note: type === 'expiration' ? 'Истечение срока годности' : type === 'manual_writeoff' ? 'Повреждена упаковка' : null }
})

export const mockStockLedger = [...mockMovements, ...Array.from({ length: 24 }, (_, index) => ({
  ...mockMovements[2], id: mockId(4000 + index), product_id: mockId(4100 + index), batch_id: mockId(5000 + index),
  product_name: `Продукт ${String(index + 1).padStart(2, '0')}`, unit: 'g' as const,
  delta_quantity: '-1.001',
})), { ...mockMovements[2], id: mockId(4300), unit: 'pcs' as const, delta_quantity: '-2.000' },
{ ...mockMovements[2], id: mockId(4301), product_id: mockId(13), batch_id: mockId(225), product_name: 'Мука', unit: 'g' as const, delta_quantity: '-99999999999.999' },
{ ...mockMovements[3], id: mockId(4302), product_id: mockId(13), batch_id: mockId(225), product_name: 'Мука', unit: 'g' as const, delta_quantity: '-99999999999.999' }]

export const mockExtraOrder: StockMovement = { ...mockMovements[1], id: mockId(4500), delta_quantity: '-999.000' }
