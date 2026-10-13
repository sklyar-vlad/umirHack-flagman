// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initialSessionState, replaceSession, saveWorkDraft, replaceBasket } from '@/entities/session'
import type { SessionController } from '@/features/auth'
import { store } from './store'
import { liveApi } from './live-tags'
import { startLiveRuntime } from './live-runtime'

class MockSource {
  static all: MockSource[] = []
  listeners = new Map<string, ((event: MessageEvent<string>) => void)[]>()
  close = vi.fn()
  constructor(public url: string, public options: EventSourceInit) { MockSource.all.push(this) }
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) { this.listeners.set(type, [...this.listeners.get(type) ?? [], listener]) }
  emit(type: string, cursor = '1', topics = ['inventory', 'batches', 'dishes']) { this.listeners.get(type)?.forEach((listener) => listener({ lastEventId: cursor, data: JSON.stringify({ occurred_at: '2026-10-13T17:00:00Z', topics }) } as MessageEvent<string>)) }
}
const read = vi.fn(async () => ({ data: 'current server data' }))
const forbidden = vi.fn(async () => ({ data: 'admin data' }))
const api = liveApi.injectEndpoints({ endpoints: (build) => ({ liveTestInventory: build.query<string, string>({ queryFn: read, providesTags: ['Inventory'] }), liveTestProducts: build.query<string, void>({ queryFn: read, providesTags: ['Products'] }), liveTestAdmin: build.query<string, void>({ queryFn: forbidden, providesTags: ['Users'] }) }) })
let dispose: (() => void) | undefined
const id = '11111111-1111-4111-8111-111111111111'
function authenticate(owner = id) { store.dispatch(replaceSession({ ...initialSessionState, status: 'authenticated', user: { id: owner, role: 'manager', name: 'Test', email: 'test@example.test' }, draftOwner: owner, expiresAt: '2099-01-01T00:00:00Z' })) }
function auth(): SessionController {
  return { restore: vi.fn(async () => true), revalidate: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), getVersion: () => 1, hasAccess: () => store.getState().session.status === 'authenticated', registerSubscription: vi.fn(() => () => true) }
}
beforeEach(() => { vi.useFakeTimers(); vi.stubGlobal('EventSource', MockSource); MockSource.all = []; read.mockClear(); forbidden.mockClear(); store.dispatch(replaceSession(initialSessionState)) })
afterEach(() => { dispose?.(); dispose = undefined; store.dispatch(api.util.resetApiState()); vi.unstubAllGlobals(); vi.useRealTimers() })
describe('session-scoped live runtime', () => {
  it('updates manager product choices while preserving the reception draft', async () => {
    authenticate(); dispose = startLiveRuntime(store, auth())
    const products = store.dispatch(api.endpoints.liveTestProducts.initiate()); await products
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Unsaved reception' } }))
    MockSource.all[0].emit('products.changed', '1', ['products'])
    await vi.advanceTimersByTimeAsync(180)
    expect(read).toHaveBeenCalledTimes(2)
    expect(store.getState().session.drafts.supply).toEqual({ note: 'Unsaved reception' })
    products.unsubscribe()
  })
  it('opens only after authentication, owns one stream and disposes on session/account/provider changes', () => {
    const controller = auth(); dispose = startLiveRuntime(store, controller)
    expect(MockSource.all).toHaveLength(0)
    authenticate(); expect(MockSource.all).toHaveLength(1)
    expect(MockSource.all[0].url).toBe('/api/v1/events'); expect(MockSource.all[0].options).toEqual({ withCredentials: true })
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Draft' } })); expect(MockSource.all).toHaveLength(1)
    authenticate('22222222-2222-4222-8222-222222222222')
    expect(MockSource.all[0].close).toHaveBeenCalledTimes(1); expect(MockSource.all).toHaveLength(2)
    store.dispatch(replaceSession(initialSessionState)); expect(MockSource.all[1].close).toHaveBeenCalledTimes(1)
    authenticate(); dispose(); expect(MockSource.all[2].close).toHaveBeenCalledTimes(1)
    expect(controller.registerSubscription).toHaveBeenCalledTimes(3)
  })
  it('re-reads only active permitted tagged queries, preserves work and invalidates the mounted check marker', async () => {
    authenticate(); dispose = startLiveRuntime(store, auth())
    const mounted = store.dispatch(api.endpoints.liveTestInventory.initiate('visible'))
    const inactive = store.dispatch(api.endpoints.liveTestInventory.initiate('inactive'))
    const admin = store.dispatch(api.endpoints.liveTestAdmin.initiate())
    await Promise.all([mounted, inactive, admin]); inactive.unsubscribe()
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Unsaved values' } })); store.dispatch(replaceBasket([{ dishId: 'dish', portions: 2 }]))
    const before = store.getState().dishChanges
    MockSource.all[0].emit('inventory.time_changed')
    await vi.advanceTimersByTimeAsync(180)
    expect(read).toHaveBeenCalledTimes(3); expect(forbidden).toHaveBeenCalledTimes(1)
    expect(store.getState().dishChanges).toBe(before + 1)
    expect(store.getState().session.drafts.supply).toEqual({ note: 'Unsaved values' })
    expect(store.getState().session.basket).toEqual([{ dishId: 'dish', portions: 2 }])
    MockSource.all[0].emit('resync.required', '5'); await vi.advanceTimersByTimeAsync(180)
    expect(read).toHaveBeenCalledTimes(4); expect(forbidden).toHaveBeenCalledTimes(1)
    expect(store.getState().session.basket).toHaveLength(1)
    mounted.unsubscribe(); admin.unsubscribe()
  })
  it('refreshes focus without needing any SSE event and ignores old-owner late delivery', async () => {
    authenticate(); const controller = auth(); dispose = startLiveRuntime(store, controller)
    const mounted = store.dispatch(api.endpoints.liveTestInventory.initiate('visible')); await mounted
    window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(180)
    expect(read).toHaveBeenCalledTimes(2); expect(controller.revalidate).toHaveBeenCalledTimes(1)
    const old = MockSource.all[0]; store.dispatch(replaceSession(initialSessionState))
    old.emit('inventory.changed', '10'); await vi.advanceTimersByTimeAsync(20_000)
    expect(read).toHaveBeenCalledTimes(2)
    mounted.unsubscribe()
  })
})
