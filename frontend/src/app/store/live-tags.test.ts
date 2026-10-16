import { describe, expect, it } from 'vitest'
import { decodeEvent } from '@/shared/api/event-contracts'
import { liveTags } from './live-tags'
function event(type: string, topics: string[]) { return decodeEvent(type, '1', JSON.stringify({ occurred_at: '2026-10-13T17:00:00Z', topics }))! }
describe('event topic cache mapping', () => {
  it.each(['manager', 'storekeeper', 'order_operator'] as const)('refreshes the published product read for %s without administrative tags', (role) => {
    expect(liveTags(role, [event('products.changed', ['products'])], false)).toEqual(role === 'manager' ? ['Products', 'StopListForecast'] : ['Products'])
    expect(liveTags(role, [], true)).toContain('Products')
  })
  it('refreshes stock ledger, losses, forecast and dependent snapshots for inventory changes', () => {
    const tags = liveTags('admin', [event('inventory.changed', ['inventory', 'stock-movements', 'orders', 'supplies', 'dishes', 'stop-list', 'procurement-recommendations', 'hot-offers'])], false)
    expect(tags).toEqual(expect.arrayContaining(['Inventory', 'Supplies', 'StockMovements', 'WriteOffReport', 'StopListForecast', 'Orders', 'OrderAttempts', 'IngredientShortages', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'AuditLog']))
  })
  it('uses the existing role matrix, including full resync without unauthorized tags', () => {
    const operator = liveTags('order_operator', [], true)
    expect(operator).toContain('Inventory'); expect(operator).toContain('Orders')
    expect(operator).not.toContain('Supplies'); expect(operator).not.toContain('StockMovements'); expect(operator).not.toContain('Users'); expect(operator).not.toContain('WriteOffReport')
    const stock = liveTags('storekeeper', [], true)
    expect(stock).toContain('StockMovements'); expect(stock).toContain('Procurement')
    expect(stock).not.toContain('Orders'); expect(stock).not.toContain('StopList'); expect(stock).not.toContain('StopListForecast')
  })
  it('time markers refresh server expiry views without inventing ledger/audit writes', () => {
    const tags = liveTags('admin', [event('inventory.time_changed', ['inventory', 'batches', 'dashboard'])], false)
    expect(tags).toContain('Batches'); expect(tags).toContain('Dashboard')
    expect(tags).not.toContain('StockMovements'); expect(tags).not.toContain('AuditLog')
  })
})
