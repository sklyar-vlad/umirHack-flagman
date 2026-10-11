import { afterEach, describe, expect, it, vi } from 'vitest'
import { initialSessionState, replaceSession, saveWorkDraft, replaceBasket, type SessionSnapshot } from '@/entities/session'
import { draftAuthAdapter } from '@/features/auth'
import { baseApi } from '@/shared/api'
import { authController } from './auth-runtime'
import { apiTransport } from './session-transport'
import { store } from './store'

const snapshot: SessionSnapshot = {
  user: { id: 'test-user', name: 'Тест', email: 'test@example.test', role: 'manager' },
  settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 },
  csrfToken: 'test-csrf', expiresAt: '2099-01-01T00:00:00Z',
}
afterEach(() => {
  vi.restoreAllMocks()
  store.dispatch(replaceSession(initialSessionState))
  store.dispatch(baseApi.util.resetApiState())
  vi.unstubAllGlobals()
})
describe('app session wiring', () => {
  it('logout clears the actual query cache, user state, drafts, basket, CSRF and subscriptions', async () => {
    vi.spyOn(draftAuthAdapter, 'login').mockResolvedValue(snapshot)
    vi.spyOn(draftAuthAdapter, 'logout').mockResolvedValue(undefined)
    await authController.login({ email: 'test@example.test', password: 'test-only' })
    await store.dispatch(baseApi.util.upsertQueryData('healthcheck', undefined, { status: 'ok', service: 'umirHack-flagman' }))
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Draft' } }))
    store.dispatch(replaceBasket([{ dishId: 'dish-1', portions: 1 }]))
    const close = vi.fn()
    authController.registerSubscription(close)
    expect(baseApi.endpoints.healthcheck.select()(store.getState()).data).toBeTruthy()
    expect(apiTransport.getCsrfToken()).toBe('test-csrf')
    await authController.logout()
    expect(baseApi.endpoints.healthcheck.select()(store.getState()).data).toBeUndefined()
    expect(store.getState().session).toMatchObject({ user: null, settings: null, expiresAt: null, drafts: {}, basket: [] })
    expect(apiTransport.getCsrfToken()).toBeNull()
    expect(close).toHaveBeenCalledTimes(1)
  })
  it('logout aborts running RTK Query requests', async () => {
    vi.spyOn(draftAuthAdapter, 'login').mockResolvedValue(snapshot)
    vi.spyOn(draftAuthAdapter, 'logout').mockResolvedValue(undefined)
    await authController.login({ email: 'test@example.test', password: 'test-only' })
    const testApi = baseApi.injectEndpoints({ endpoints: (build) => ({ pendingCleanupProbe: build.query<unknown, void>({ query: () => 'http://localhost/api/v1/inventory' }) }) })
    let signal: AbortSignal | undefined
    let started!: () => void
    const ready = new Promise<void>((resolve) => { started = resolve })
    vi.stubGlobal('fetch', vi.fn((request: Request) => new Promise<Response>((_resolve, reject) => {
      signal = request.signal
      request.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
      started()
    })))
    const request = store.dispatch(testApi.endpoints.pendingCleanupProbe.initiate())
    await ready
    await authController.logout()
    expect(signal?.aborted).toBe(true)
    await request
    expect(testApi.endpoints.pendingCleanupProbe.select()(store.getState()).data).toBeUndefined()
    request.unsubscribe()
  })
})
