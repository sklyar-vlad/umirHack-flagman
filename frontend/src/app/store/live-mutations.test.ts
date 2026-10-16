// @vitest-environment jsdom
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { initialSessionState, replaceSession } from '@/entities/session'
import { supplyApi } from '@/entities/supply'
import { orderApi } from '@/entities/order'
import { liveApi } from './live-tags'
import { store } from './store'
import { receptionHandlers } from '../mocks/reception-handlers'
import { orderCompositionHandlers } from '../mocks/order-composition-handlers'
import { inventoryItems } from '../mocks/inventory-fixtures'
import { compositionDishes, resetCompositionFixtures } from '../mocks/order-composition-fixtures'
import { resetReceptionFixtures } from '../mocks/reception-fixtures'
import { setMockScenario } from '../mocks/scenarios'

const read = vi.fn(async () => ({ data: 'server ledger' }))
const api = liveApi.injectEndpoints({ endpoints: (build) => ({ mutationTestLedger: build.query<string, void>({ queryFn: read, providesTags: ['StockMovements'] }) }) })
const server = setupServer(...receptionHandlers, ...orderCompositionHandlers)
const NativeRequest = globalThis.Request
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
})
beforeEach(() => {
  read.mockClear(); setMockScenario('success'); resetCompositionFixtures(); resetReceptionFixtures()
  store.dispatch(replaceSession({ ...initialSessionState, status: 'authenticated', user: { id: '11111111-1111-4111-8111-111111111111', name: 'Test', email: 'test@example.test', role: 'manager' }, expiresAt: '2099-01-01T00:00:00Z' }))
})
afterEach(() => { store.dispatch(api.util.resetApiState()); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
describe('writes refresh without SSE delivery', () => {
  it('confirmed reception refreshes active stock movements and makes old basket checks stale', async () => {
    const ledger = store.dispatch(api.endpoints.mutationTestLedger.initiate()); await ledger
    const before = store.getState().dishChanges
    const receivedAt = new Date().toISOString(), expiresAt = new Date(Date.now() + 86400_000).toISOString()
    await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F30-SUPPLY', received_at: receivedAt, items: [{ product_id: inventoryItems[0].product.id, quantity: '1.000', expires_at: expiresAt }] } })).unwrap()
    await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    expect(store.getState().dishChanges).toBe(before + 1)
    ledger.unsubscribe()
  })
  it('whole-basket checks neither invalidate themselves nor refresh the ledger', async () => {
    const ledger = store.dispatch(api.endpoints.mutationTestLedger.initiate()); await ledger
    const before = store.getState().dishChanges
    await store.dispatch(orderApi.endpoints.checkOrder.initiate({ items: [{ dish_id: compositionDishes()[0].id, portions: 1, recipe_version: 1 }] })).unwrap()
    expect(store.getState().dishChanges).toBe(before); expect(read).toHaveBeenCalledTimes(1)
    ledger.unsubscribe()
  })
  it('uncertain writes invalidate old checks while leaving replay ownership to the form', async () => {
    setMockScenario('reception-lost-response')
    const before = store.getState().dishChanges
    const receivedAt = new Date().toISOString(), expiresAt = new Date(Date.now() + 86400_000).toISOString()
    await expect(store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F30-UNKNOWN', received_at: receivedAt, items: [{ product_id: inventoryItems[0].product.id, quantity: '1.000', expires_at: expiresAt }] } })).unwrap()).rejects.toBeDefined()
    expect(store.getState().dishChanges).toBe(before + 1)
  })
})
