import type { SupplyDetail } from '@/entities/supply'
import type { CreateSupplyRequest } from '@/shared/api'
import { inventoryItems } from './inventory-fixtures'

export const receptionProducts = inventoryItems.filter((item) => item.product.is_active).map(({ product }) => ({
  ...product, standard_shelf_life_hours: 48, target_stock: null, revision: 1, created_at: '2026-01-01T00:00:00Z',
}))
interface SavedReception { key: string; body: string; supply: SupplyDetail }
let saved: SavedReception[] = []
const storageKey = 'flagman.mock.received.v1'
function read(): SavedReception[] {
  try { saved = JSON.parse(sessionStorage.getItem(storageKey) ?? '[]') as SavedReception[] } catch { /* Node tests use memory. */ }
  return saved
}
export function receivedSupplies() { return read().map((item) => item.supply) }
export function savedReception(key: string) { return read().find((item) => item.key === key) }
export function resetReceptionFixtures() { saved = []; try { sessionStorage.removeItem(storageKey) } catch { /* Node tests. */ } }
export function acceptReception(key: string, body: CreateSupplyRequest): SupplyDetail {
  const now = new Date().toISOString(), id = crypto.randomUUID()
  const supply: SupplyDetail = {
    id, number: body.number.trim(), status: 'received', created_at: now, received_at: body.received_at,
    created_by: { id: '11111111-1111-4111-8111-111111111111', name: 'Тестовый сотрудник' }, items_count: body.items.length, meta: { server_time: now },
    items: body.items.map((line, index) => {
      const product = receptionProducts.find((item) => item.id === line.product_id)!
      const seconds = Math.floor((Date.parse(line.expires_at) - Date.parse(now)) / 1000)
      return { product, batch: { id: crypto.randomUUID(), product_id: product.id, unit: product.unit, supply: { id, number: body.number.trim(), received_at: body.received_at }, line_number: index + 1,
        initial_quantity: line.quantity, remaining_quantity: line.quantity, produced_at: null, shelf_life_hours: null, expiry_source: 'explicit', expires_at: line.expires_at, remaining_seconds: seconds, status: seconds <= 0 ? 'expired' : seconds <= 86400 ? 'expiring' : 'valid', created_at: now } }
    }),
  }
  saved.push({ key, body: JSON.stringify(body), supply })
  try { sessionStorage.setItem(storageKey, JSON.stringify(saved)) } catch { /* Node tests. */ }
  return supply
}
