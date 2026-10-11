import { configureStore } from '@reduxjs/toolkit'
import { describe, expect, it } from 'vitest'
import { initialSessionState, replaceSession, sessionReducer, saveWorkDraft, clearWorkDraft, type SessionState } from '@/entities/session'
import { persistOrder } from './order-persistence'
import type { AppStore } from './store'
import { basketDishSchema } from '@/shared/api/order-work'
import { compositionDishes } from '@/app/mocks/order-composition-fixtures'
const emptyOrder = () => ({ lines: [{ dish: basketDishSchema.parse(compositionDishes()[0]), portions: '2' }] })

const owner = '11111111-1111-4111-8111-111111111111'
function authenticated(id = owner): SessionState { return { ...initialSessionState, status: 'authenticated', draftOwner: id, user: { id, name: 'Тест', email: 'test@example.test', role: 'manager' } } }
function setup(memory = new Map<string, string>()) {
  const store = configureStore({ reducer: { session: sessionReducer } })
  const storage = { getItem: (key: string) => memory.get(key) ?? null, setItem: (key: string, value: string) => { memory.set(key, value) }, removeItem: (key: string) => { memory.delete(key) } }
  persistOrder(store as unknown as AppStore, storage)
  return { store, memory }
}
describe('owner-scoped order persistence', () => {
  it('rehydrates only after authentication, survives expiration/reload and clears on success', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated()))
    const attempt = { key: crypto.randomUUID(), startedAt: Date.now(), body: { items: [{ dish_id: compositionDishes()[0].id, portions: 2, recipe_version: 1 }] } }
    first.store.dispatch(saveWorkDraft({ key: 'order', draft: { order: { ...emptyOrder(), attempt }, ...{ password: 'never-store', csrfToken: 'never-store' } } }))
    expect([...first.memory.values()][0]).not.toContain('never-store')
    const second = setup(first.memory)
    expect(second.store.getState().session.drafts).toEqual({})
    second.store.dispatch(replaceSession(authenticated()))
    expect(second.store.getState().session.drafts.order.order?.lines).toEqual(emptyOrder().lines)
    expect(second.store.getState().session.drafts.order.order?.attempt).toEqual(attempt)
    expect(JSON.stringify(second.store.getState().session.drafts.order.order?.attempt?.body)).toBe(JSON.stringify(attempt.body))
    const state = second.store.getState().session
    second.store.dispatch(replaceSession({ ...initialSessionState, status: 'anonymous', draftOwner: owner, drafts: state.drafts }))
    expect(second.memory.size).toBe(1)
    second.store.dispatch(replaceSession({ ...authenticated(), drafts: state.drafts }))
    expect(second.store.getState().session.basket).toEqual([{ dishId: compositionDishes()[0].id, portions: 2 }])
    second.store.dispatch(clearWorkDraft('order'))
    expect(second.store.getState().session.basket).toEqual([])
    expect(second.memory.size).toBe(0)
  })
  it('erases another owner and explicit logout; malformed storage is ignored', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated()))
    first.store.dispatch(saveWorkDraft({ key: 'order', draft: { order: { ...emptyOrder() } } }))
    const other = setup(first.memory); other.store.dispatch(replaceSession(authenticated('22222222-2222-4222-8222-222222222222')))
    expect(other.store.getState().session.drafts).toEqual({}); expect(other.memory.size).toBe(0)
    first.store.dispatch(saveWorkDraft({ key: 'order', draft: { order: { ...emptyOrder() } } }))
    first.store.dispatch(replaceSession({ ...initialSessionState, status: 'signing-out' }))
    expect(first.memory.size).toBe(0)
    const corrupt = setup(new Map([['flagman.order.v1', 'not-json']]))
    expect(() => corrupt.store.dispatch(replaceSession(authenticated()))).not.toThrow()
  })
})
