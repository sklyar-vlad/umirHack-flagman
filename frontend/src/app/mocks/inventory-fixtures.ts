import type { Batch, Category, InventoryItem } from '@/entities/inventory'

export const inventoryServerTime = '2026-10-11T12:00:00Z'
export const mockId = (number: number) => `00000000-0000-4000-8000-${String(number).padStart(12, '0')}`
export const inventoryCategories: Category[] = [
  { id: mockId(1), name: 'Молочные продукты', revision: 1 },
  { id: mockId(2), name: 'Бакалея', revision: 1 },
  { id: mockId(3), name: 'Штучные продукты', revision: 1 },
]
const supply = { id: mockId(100), number: 'ТЕСТ-101', received_at: '2026-10-10T09:00:00Z' }
function batch(productId: string, number: number, line: number, quantity: string, seconds: number, status: Batch['status'], unit: Batch['unit'] = 'ml'): Batch {
  return {
    id: mockId(number), product_id: productId, unit, supply, line_number: line,
    initial_quantity: quantity === '0.000' ? '100.000' : quantity, remaining_quantity: quantity,
    produced_at: null, shelf_life_hours: null, expiry_source: 'explicit',
    expires_at: new Date(Date.parse(inventoryServerTime) + seconds * 1000).toISOString(),
    remaining_seconds: seconds, status, created_at: '2026-10-10T09:00:00Z',
  }
}
const milkId = mockId(10), expiredId = mockId(11), emptyId = mockId(12), flourId = mockId(13), eggsId = mockId(14)
export const inventoryBatches: Batch[] = [
  batch(milkId, 201, 1, '200.000', -3600, 'expired'),
  batch(milkId, 202, 2, '500.000', 64800, 'expiring'),
  ...Array.from({ length: 20 }, (_, index) => batch(milkId, 203 + index, index + 3, '50.000', 172800 + index, 'valid')),
  batch(milkId, 223, 23, '0.000', -86400, 'depleted'),
  batch(expiredId, 224, 24, '300.000', 0, 'expired', 'g'),
  batch(flourId, 225, 25, '99999999999.999', 259200, 'valid', 'g'),
  batch(flourId, 226, 26, '99999999999.999', 345600, 'valid', 'g'),
  batch(eggsId, 227, 27, '10.000', 172800, 'valid', 'pcs'),
]
function item(id: string, name: string, unit: Batch['unit'], display: InventoryItem['product']['display_unit'], category: Category, quantities: Partial<InventoryItem>): InventoryItem {
  return {
    product: { id, name, unit, display_unit: display, category: { id: category.id, name: category.name }, is_active: true },
    physical_quantity: '0.000', available_quantity: '0.000', expiring_quantity: '0.000', expired_quantity: '0.000',
    target_stock: null, deficit_quantity: null, batches_count: 0, nearest_expires_at: null, remaining_seconds: null,
    has_expiring: false, has_expired: false, is_out_of_stock: true, status: 'out_of_stock', ...quantities,
  }
}
export const inventoryItems: InventoryItem[] = [
  item(milkId, 'Молоко', 'ml', 'l', inventoryCategories[0], {
    physical_quantity: '1700.000', available_quantity: '1500.000', expiring_quantity: '500.000', expired_quantity: '200.000',
    target_stock: '4000.000', deficit_quantity: '2500.000', batches_count: 22, nearest_expires_at: inventoryBatches[1].expires_at,
    remaining_seconds: 64800, has_expiring: true, has_expired: true, is_out_of_stock: false, status: 'expired',
  }),
  item(expiredId, 'Творог — только просрочка', 'g', 'kg', inventoryCategories[0], {
    physical_quantity: '300.000', expired_quantity: '300.000', batches_count: 1, has_expired: true, status: 'expired',
    product: { id: expiredId, name: 'Творог — только просрочка', unit: 'g', display_unit: 'kg', category: { id: inventoryCategories[0].id, name: inventoryCategories[0].name }, is_active: false },
  }),
  item(emptyId, 'Сахар', 'g', 'kg', inventoryCategories[1], {}),
  item(flourId, 'Мука', 'g', 'kg', inventoryCategories[1], {
    physical_quantity: '199999999999.998', available_quantity: '199999999999.998', batches_count: 2,
    nearest_expires_at: inventoryBatches.find((batch) => batch.product_id === flourId)!.expires_at, remaining_seconds: 259200, is_out_of_stock: false, status: 'normal',
  }),
  item(eggsId, 'Яйцо', 'pcs', 'pcs', inventoryCategories[2], {
    physical_quantity: '10.000', available_quantity: '10.000', batches_count: 1, nearest_expires_at: inventoryBatches.find((batch) => batch.product_id === eggsId)!.expires_at,
    remaining_seconds: 172800, is_out_of_stock: false, status: 'normal', target_stock: '20.000', deficit_quantity: '10.000',
  }),
  ...Array.from({ length: 20 }, (_, index) => item(mockId(300 + index), `Крупа ${String(index + 1).padStart(2, '0')}`, 'g', 'kg', inventoryCategories[1], {})),
]
