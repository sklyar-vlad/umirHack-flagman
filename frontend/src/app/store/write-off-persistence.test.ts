import { configureStore } from '@reduxjs/toolkit'
import { describe, expect, it } from 'vitest'
import { initialSessionState, replaceSession, sessionReducer, saveWorkDraft, clearWorkDraft, type SessionState } from '@/entities/session'
import { inventoryBatches, inventoryItems, mockId } from '@/app/mocks/inventory-fixtures'
import { persistWriteOff } from './write-off-persistence'
import type { AppStore } from './store'

const owner = mockId(900)
function authenticated(id = owner): SessionState { return { ...initialSessionState, status: 'authenticated', draftOwner: id, user: { id, name: 'Тест', email: 'test@example.test', role: 'manager' } } }
function setup(memory = new Map<string, string>()) {
  const store = configureStore({ reducer: { session: sessionReducer } })
  persistWriteOff(store as unknown as AppStore, { getItem: (key) => memory.get(key) ?? null, setItem: (key, value) => { memory.set(key, value) }, removeItem: (key) => { memory.delete(key) } })
  return { store, memory }
}
const draft = { batch: inventoryBatches[0], product: inventoryItems[0].product, values: { quantity: '0.001125', reason: 'Порча' }, attempt: { key: crypto.randomUUID(), startedAt: Date.now(), body: { batch_id: inventoryBatches[0].id, quantity: '1.125', reason: 'Порча' } } }
describe('write-off ownership and immutable request persistence', () => {
  it('restores only after same-owner authentication, retains exact key/body across expiration and clears after success', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated()))
    first.store.dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: { ...draft, ...{ password: 'never-store' } } } }))
    expect([...first.memory.values()][0]).not.toContain('never-store')
    first.store.dispatch(replaceSession({ ...initialSessionState, status: 'anonymous', draftOwner: owner, drafts: first.store.getState().session.drafts }))
    const second = setup(first.memory); expect(second.store.getState().session.drafts).toEqual({})
    second.store.dispatch(replaceSession(authenticated()))
    expect(second.store.getState().session.drafts.writeOff.writeOff).toEqual({ ...draft, product: { id: draft.product.id, name: draft.product.name, unit: draft.product.unit, display_unit: draft.product.display_unit } })
    second.store.dispatch(clearWorkDraft('writeOff')); expect(second.memory.size).toBe(0)
  })
  it('removes other-owner data and explicit logout data', () => {
    const first = setup(); first.store.dispatch(replaceSession(authenticated())); first.store.dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: draft } }))
    const other = setup(first.memory); other.store.dispatch(replaceSession(authenticated(mockId(901)))); expect(other.store.getState().session.drafts).toEqual({}); expect(other.memory.size).toBe(0)
    first.store.dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: draft } })); first.store.dispatch(replaceSession({ ...authenticated(), status: 'signing-out' })); expect(first.memory.size).toBe(0)
  })
  it('rejects mismatched batch/product/request identity before storage', () => {
    const { store, memory } = setup(); store.dispatch(replaceSession(authenticated()))
    store.dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: { ...draft, attempt: { ...draft.attempt, body: { ...draft.attempt.body, batch_id: mockId(1234) } } } } }))
    expect(store.getState().session.drafts.writeOff.writeOff).toBeUndefined(); expect(memory.size).toBe(0)
  })
})
