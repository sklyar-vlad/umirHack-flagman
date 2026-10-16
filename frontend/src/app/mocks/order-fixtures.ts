import type { OrderSummary, OrderAttempt } from '@/entities/order'
const uuid = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
const actor = { id: uuid(900), name: 'Тестовый оператор' }
const items = [{ dish_id: uuid(800), dish_name: 'Исторический суп', portions: 2, recipe_version: 3 }, { dish_id: uuid(801), dish_name: 'Салат до переименования', portions: 1, recipe_version: 1 }]
const order = (n: number, number: string, created_at: string): OrderSummary => ({ id: uuid(n), number, status: 'completed', created_at, created_by: actor, items, total_portions: 3 })
export const orderFixtures: OrderSummary[] = [
  order(100, 'ТЕСТ-101', '2026-10-10T09:05:00Z'),
  order(101, 'ГРАНИЦА-С', '2026-10-09T21:00:00Z'), order(102, 'ГРАНИЦА-ДО', '2026-10-10T21:00:00Z'),
  order(103, 'Буквальный %_\\', '2026-10-08T12:00:00.123456789Z'),
  ...Array.from({ length: 22 }, (_, i) => order(110 + i, `ИСТОРИЯ-${i + 1}`, '2026-10-07T10:00:00Z')),
]
const shortage: OrderAttempt = {
  id: uuid(200), created_at: '2026-10-10T09:10:00Z', created_by: actor, reason_code: 'INSUFFICIENT_STOCK', items,
  shortages: [{ product_id: uuid(700), product_name: 'Мука из снимка', unit: 'g', required_quantity: '1200.001', available_quantity: '0.000', missing_quantity: '1200.001', expired_quantity: '500.000', reason_code: 'only_expired_stock' }], issues: [],
}
export const attemptFixtures: OrderAttempt[] = [shortage,
  { ...shortage, id: uuid(201), created_at: '2026-10-10T09:09:00Z', reason_code: 'RECIPE_CHANGED', shortages: [], issues: [{ dish_id: items[0].dish_id, code: 'recipe_changed', message: 'Рецептура изменилась после проверки', current_recipe_version: 4 }] },
  { ...shortage, id: uuid(202), created_at: '2026-10-10T09:08:00Z', reason_code: 'DISH_UNAVAILABLE', shortages: [], issues: [{ dish_id: items[1].dish_id, code: 'dish_inactive', message: 'Блюдо отключено', current_recipe_version: null }] },
  ...Array.from({ length: 22 }, (_, i) => ({ ...shortage, id: uuid(210 + i), created_at: '2026-10-07T10:00:00Z' })),
]
