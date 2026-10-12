import { configureStore } from '@reduxjs/toolkit'
import { describe, expect, it } from 'vitest'
import { initialSessionState, replaceSession, sessionReducer, saveWorkDraft, clearWorkDraft, type SessionState } from '@/entities/session'
import { persistDishes } from './dish-persistence'
import type { AppStore } from './store'
import { emptyDish } from '@/features/manage-dish'

const owner = '11111111-1111-4111-8111-111111111111'
function authenticated(id = owner): SessionState { return { ...initialSessionState, status: 'authenticated', draftOwner: id, user: { id, name: 'Тест', email: 'test@example.test', role: 'manager' } } }
function setup(memory = new Map<string, string>()) {
  const store = configureStore({ reducer: { session: sessionReducer } })
  const storage = { getItem: (key: string) => memory.get(key) ?? null, setItem: (key: string, value: string) => { memory.set(key, value) }, removeItem: (key: string) => { memory.delete(key) } }
  persistDishes(store as unknown as AppStore, storage)
  return { store, memory }
}
const draft = { resourceId: 'new' as const, values: { ...emptyDish, name: 'Черновик' }, baseline: null, uncertain: true, review: false }
const save = () => saveWorkDraft({ key: 'dish:new', draft: { dish: draft, ...{ password: 'never-store', csrfToken: 'never-store' } } })
describe('owner-scoped dish draft persistence', () => {
  it('restores only after same-owner authentication and clears after success', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated())); first.store.dispatch(save())
    expect([...first.memory.values()][0]).not.toContain('never-store')
    const second = setup(first.memory); expect(second.store.getState().session.drafts).toEqual({})
    second.store.dispatch(replaceSession(authenticated())); expect(second.store.getState().session.drafts['dish:new'].dish).toEqual(draft)
    const state = second.store.getState().session
    second.store.dispatch(replaceSession({ ...initialSessionState, status: 'anonymous', draftOwner: owner, drafts: state.drafts })); expect(second.memory.size).toBe(1)
    second.store.dispatch(replaceSession({ ...authenticated(), drafts: state.drafts })); second.store.dispatch(clearWorkDraft('dish:new')); expect(second.memory.size).toBe(0)
  })
  it('clears another owner and explicit logout; strips unexpected fields', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated())); first.store.dispatch(save())
    const other = setup(first.memory); other.store.dispatch(replaceSession(authenticated('22222222-2222-4222-8222-222222222222')))
    expect(other.store.getState().session.drafts).toEqual({}); expect(other.memory.size).toBe(0)
    first.store.dispatch(save()); first.store.dispatch(replaceSession({ ...initialSessionState, status: 'signing-out' })); expect(first.memory.size).toBe(0)
    const corrupt = setup(new Map([['flagman.dishes.v1', 'not-json']])); expect(() => corrupt.store.dispatch(replaceSession(authenticated()))).not.toThrow()
  })
})
