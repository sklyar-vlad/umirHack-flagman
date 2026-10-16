import type { SupplySummary } from '@/entities/supply'

const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
const actor = { id: id(900), name: 'Тестовый кладовщик' }
export const supplyFixtures: SupplySummary[] = [
  { id: id(100), number: 'ТЕСТ-101', created_at: '2026-10-10T09:05:00Z', received_at: '2026-10-10T09:00:00Z', items_count: 27, created_by: actor, status: 'received' },
  { id: id(601), number: 'ГРАНИЦА-С', created_at: '2026-10-09T21:00:00Z', received_at: '2026-10-08T10:00:00Z', items_count: 1, created_by: actor, status: 'received' },
  { id: id(602), number: 'ГРАНИЦА-ДО', created_at: '2026-10-10T21:00:00Z', received_at: '2026-10-10T12:00:00Z', items_count: 2, created_by: actor, status: 'received' },
  { id: id(603), number: 'Буквальный %_\\', created_at: '2026-10-08T12:00:00.123456789Z', received_at: '2026-10-08T11:00:00Z', items_count: 100, created_by: actor, status: 'received' },
  ...Array.from({ length: 22 }, (_, index): SupplySummary => ({ id: id(610 + index), number: `ИСТОРИЯ-${index + 1}`, created_at: '2026-10-01T09:00:00Z', received_at: '2026-09-30T09:00:00Z', items_count: index + 1, created_by: actor, status: 'received' })),
]
export const newlyReceivedSupply: SupplySummary = { id: id(700), number: 'НОВАЯ-ТЕСТОВАЯ', created_at: '2026-10-11T12:00:00Z', received_at: '2026-10-11T11:55:00Z', items_count: 3, created_by: actor, status: 'received' }
