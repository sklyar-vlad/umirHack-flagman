import { configureStore } from '@reduxjs/toolkit'
import { describe, expect, it, vi } from 'vitest'
import { sessionReducer, replaceSession, saveWorkDraft, replaceBasket, type SessionSnapshot } from '@/entities/session'
import type { AuthAdapter } from '../api/draft-auth-adapter'
import { createSessionController } from './session-controller'

const snapshot: SessionSnapshot = { user: { id: '11111111-1111-4111-8111-111111111111', email: 'employee@example.test', name: 'Сотрудник', role: 'manager' }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 }, csrfToken: 'test-csrf', expiresAt: '2099-01-01T00:00:00Z' }
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((ok, fail) => { resolve = ok; reject = fail })
  return { promise, resolve, reject }
}
function setup(overrides: Partial<AuthAdapter> = {}) {
  const store = configureStore({ reducer: { session: sessionReducer } })
  const adapter: AuthAdapter = { me: vi.fn(async () => snapshot), login: vi.fn(async () => snapshot), logout: vi.fn(async () => {}), ...overrides }
  const setCsrf = vi.fn()
  const clearApi = vi.fn()
  const auth = createSessionController(adapter, { read: () => store.getState().session, write: (state) => { store.dispatch(replaceSession(state)) }, setCsrf, clearApi })
  return { store, adapter, auth, setCsrf, clearApi }
}
const credentials = { email: snapshot.user.email, password: 'test-secret' }

describe('session lifecycle', () => {
  it('restores once before access and supplies user, settings and memory-only CSRF', async () => {
    const response = deferred<SessionSnapshot>()
    const { auth, store, adapter, setCsrf } = setup({ me: vi.fn(() => response.promise) })
    const first = auth.restore()
    expect(store.getState().session.status).toBe('restoring')
    expect(auth.restore()).toBe(first)
    expect(adapter.me).toHaveBeenCalledTimes(1)
    response.resolve(snapshot)
    expect(await first).toBe(true)
    expect(store.getState().session.user).toEqual(snapshot.user)
    expect(store.getState().session.settings).toEqual(snapshot.settings)
    expect(setCsrf).toHaveBeenLastCalledWith('test-csrf')
    expect(JSON.stringify(store.getState())).not.toContain('csrf')
  })
  it('treats missing and disabled sessions as anonymous without automatic retry', async () => {
    for (const status of [401, 403]) {
      const { auth, adapter, store } = setup({ me: vi.fn(async () => { throw { status, code: 'DENIED', message: '', details: {} } }) })
      expect(await auth.restore()).toBe(false)
      expect(await auth.restore()).toBe(false)
      expect(adapter.me).toHaveBeenCalledTimes(1)
      expect(store.getState().session.status).toBe('anonymous')
    }
  })
  it('allows manual retry after a transport failure while keeping access closed', async () => {
    const me = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(snapshot)
    const { auth, store } = setup({ me })
    expect(await auth.restore()).toBe(false)
    expect(store.getState().session.status).toBe('error')
    expect(await auth.restore()).toBe(true)
  })
  it('preserves allowlisted drafts on expiration for the same user, clearing private data and resources', async () => {
    const { auth, store, clearApi, setCsrf } = setup()
    await auth.login(credentials)
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Хранить охлаждённым', title: 'Поставка', ...{ password: 'must-not-survive' } } }))
    store.dispatch(replaceBasket([{ dishId: 'dish-1', portions: 2 }]))
    const close = vi.fn()
    auth.registerSubscription(close)
    auth.expire()
    expect(store.getState().session.user).toBeNull()
    expect(store.getState().session.settings).toBeNull()
    expect(store.getState().session.basket).toEqual([])
    expect(setCsrf).toHaveBeenLastCalledWith(null)
    expect(clearApi).toHaveBeenCalledTimes(1)
    expect(close).toHaveBeenCalledTimes(1)
    expect(JSON.stringify(store.getState())).not.toContain('must-not-survive')
    auth.expire()
    expect(close).toHaveBeenCalledTimes(1)
    await auth.login(credentials)
    expect(store.getState().session.drafts.supply).toEqual({ title: 'Поставка', note: 'Хранить охлаждённым' })
    expect(JSON.stringify(store.getState())).not.toContain('test-secret')
  })
  it('discards previous-user drafts on account switch', async () => {
    const login = vi.fn().mockResolvedValueOnce(snapshot).mockResolvedValueOnce({ ...snapshot, user: { ...snapshot.user, id: '22222222-2222-4222-8222-222222222222' } })
    const { auth, store } = setup({ login })
    await auth.login(credentials)
    store.dispatch(saveWorkDraft({ key: 'supply', draft: { note: 'Private draft' } }))
    auth.expire()
    await auth.login(credentials)
    expect(store.getState().session.drafts).toEqual({})
  })
  it('logout immediately purges data and subscriptions; CSRF accompanies only the outgoing request', async () => {
    const response = deferred<void>()
    const logout = vi.fn(() => response.promise)
    const { auth, store, setCsrf, clearApi } = setup({ logout })
    await auth.login(credentials)
    store.dispatch(saveWorkDraft({ key: 'order', draft: { note: 'draft' } }))
    store.dispatch(replaceBasket([{ dishId: 'dish', portions: 3 }]))
    const close = vi.fn(() => { throw new Error('disposer failed') })
    const second = vi.fn()
    auth.registerSubscription(close)
    auth.registerSubscription(second)
    const done = auth.logout()
    expect(store.getState().session).toMatchObject({ status: 'signing-out', user: null, settings: null, drafts: {}, basket: [] })
    expect(setCsrf).toHaveBeenLastCalledWith(null)
    expect(logout).toHaveBeenCalledWith('test-csrf', expect.any(AbortSignal))
    expect(second).toHaveBeenCalledTimes(1)
    expect(clearApi).toHaveBeenCalledTimes(1)
    expect(await auth.restore()).toBe(false)
    response.resolve()
    await done
    expect(store.getState().session.status).toBe('anonymous')
  })
  it('does not resurrect a session when restore resolves after logout', async () => {
    const response = deferred<SessionSnapshot>()
    const { auth, store } = setup({ me: () => response.promise })
    const restore = auth.restore()
    await auth.logout()
    response.resolve(snapshot)
    expect(await restore).toBe(false)
    expect(store.getState().session.user).toBeNull()
  })
  it('rejects a stale login result after logout', async () => {
    const response = deferred<SessionSnapshot>()
    const { auth, store } = setup({ login: () => response.promise })
    const login = auth.login(credentials)
    await auth.logout()
    response.resolve(snapshot)
    await expect(login).rejects.toMatchObject({ code: 'SESSION_CHANGED' })
    expect(store.getState().session.user).toBeNull()
  })
  it('reports unconfirmed server logout while preserving local cleanup', async () => {
    const { auth, store } = setup({ logout: async () => { throw new Error('offline') } })
    await auth.login(credentials)
    await auth.logout()
    expect(store.getState().session.user).toBeNull()
    expect(store.getState().session.notice).toContain('Сервер не подтвердил выход')
  })
})
