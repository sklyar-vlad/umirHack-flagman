import type { Batch } from '@/entities/inventory'
import type { StockMovement } from '@/entities/stock-movement'
import { parseMovementQuantity, parseQuantity, serializeQuantity } from '@/shared/lib/quantity'

interface SavedWriteOff { key: string; body: string; status: number; payload: unknown; movement?: StockMovement; expired?: boolean }
let saved: SavedWriteOff[] = []
const storageKey = 'flagman.mock.write-offs.v1'
function read() { try { saved = JSON.parse(sessionStorage.getItem(storageKey) ?? '[]') as SavedWriteOff[] } catch { /* Node tests use memory. */ } return saved }
export function saveMockWriteOff(value: SavedWriteOff) { saved = [...read(), value]; try { sessionStorage.setItem(storageKey, JSON.stringify(saved)) } catch { /* Node tests use memory. */ } }
export function savedMockWriteOff(key: string) { return read().find((item) => item.key === key) }
export function writtenOffMovements() { return read().flatMap((item) => item.movement ? [item.movement] : []) }
export function removedWriteOffQuantity(productId: string, expired?: boolean) {
  return read().filter((item) => item.movement?.product_id === productId && (expired === undefined || item.expired === expired)).reduce((sum, item) => sum - parseMovementQuantity(item.movement!.delta_quantity)!, 0n)
}
export function applyBatchWriteOff(batch: Batch): Batch {
  const removed = writtenOffMovements().filter((item) => item.batch_id === batch.id).reduce((sum, item) => sum - parseMovementQuantity(item.delta_quantity)!, 0n)
  const remaining = parseQuantity(batch.remaining_quantity)! - removed
  return removed ? { ...batch, remaining_quantity: serializeQuantity(remaining > 0n ? remaining : 0n)!, status: remaining <= 0n ? 'depleted' : batch.status } : batch
}
export function resetWriteOffFixtures() { saved = []; try { sessionStorage.removeItem(storageKey) } catch { /* Node tests. */ } }
