import type { ShortageRow, ShortageSearch } from '@/entities/ingredient-shortage'
import type { OrderAttempt } from '@/entities/order'
import { compareHistoryInstants } from '@/shared/lib/history'
import { composedAttempts } from './order-composition-fixtures'
import { attemptFixtures } from './order-fixtures'
import { mockId } from './inventory-fixtures'

const sample = attemptFixtures[0]
const demo = (id: number, productId: number, name: string, unit: 'g' | 'ml' | 'pcs', amount: string, time = '2026-10-10T09:00:00Z'): OrderAttempt => ({
  ...sample, id: mockId(id), created_at: time,
  shortages: [{ ...sample.shortages[0], product_id: mockId(productId), product_name: name, unit, required_quantity: amount, missing_quantity: amount }],
})
export const shortageDemoAttempts = [
  demo(2200, 700, 'Мука: последнее историческое имя', 'g', '99999999999.999'),
  demo(2201, 700, 'Мука в прежней единице', 'ml', '3200.001'),
  demo(2202, 700, 'Мука: последнее историческое имя', 'g', '99999999999.999', '2026-10-10T09:01:00Z'),
  ...Array.from({ length: 25 }, (_, i) => demo(2210 + i, 2300 + i, `Исторический продукт ${i + 1}`, i === 0 ? 'pcs' : 'g', i === 0 ? '2.000' : '0.001')),
]
export function shortageAttempts(demo = false) { return [...attemptFixtures, ...(demo ? shortageDemoAttempts : []), ...composedAttempts()] }

export function shortageRows(values: ShortageSearch, attempts: OrderAttempt[]): ShortageRow[] {
  const groups = new Map<string, { row: ShortageRow; amount: bigint; ids: Set<string>; latest: string }>()
  const distinct = new Map(attempts.map((attempt) => [attempt.id.toLowerCase(), attempt]))
  for (const attempt of distinct.values()) {
    if (attempt.reason_code !== 'INSUFFICIENT_STOCK' || values.from && compareHistoryInstants(attempt.created_at, values.from) < 0 || values.to && compareHistoryInstants(attempt.created_at, values.to) >= 0) continue
    for (const line of attempt.shortages) {
      const key = `${line.product_id.toLowerCase()}:${line.unit}`
      const group = groups.get(key) ?? { row: { product_id: line.product_id, product_name: line.product_name, unit: line.unit, rejected_attempts_count: 0, total_missing_quantity: '0.000' }, amount: 0n, ids: new Set<string>(), latest: attempt.created_at }
      const [whole, fraction = ''] = line.missing_quantity.split('.')
      group.amount += BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0'))
      group.ids.add(attempt.id.toLowerCase())
      if (compareHistoryInstants(attempt.created_at, group.latest) >= 0) { group.row.product_name = line.product_name; group.latest = attempt.created_at }
      groups.set(key, group)
    }
  }
  return [...groups.values()].map(({ row, amount, ids }) => ({ ...row, rejected_attempts_count: ids.size, total_missing_quantity: `${amount / 1000n}.${String(amount % 1000n).padStart(3, '0')}` }))
    .sort((a, b) => b.rejected_attempts_count - a.rejected_attempts_count || a.product_id.localeCompare(b.product_id) || a.unit.localeCompare(b.unit))
}
