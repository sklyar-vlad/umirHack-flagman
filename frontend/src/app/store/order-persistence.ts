import { z } from 'zod'
import { orderDraftSchema } from '@/shared/api/order-work'
import { saveWorkDraft } from '@/entities/session'
import type { AppStore } from './store'

const storageKey = 'flagman.order.v1'
const storedSchema = z.object({ owner: z.uuid(), order: orderDraftSchema })
/** Tab-local, non-secret data only. Ownership is checked after real session restoration. */
export function persistOrder(store: AppStore, storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
  let restored = false, writing = false
  return store.subscribe(() => {
    if (writing) return
    const state = store.getState().session
    if (state.status === 'signing-out') { restored = true; try { storage.removeItem(storageKey) } catch { /* Storage can be unavailable. Memory still preserves the attempt. */ } return }
    if (state.status !== 'authenticated' || !state.user) return
    if (!restored) {
      restored = true
      try {
        const saved = storedSchema.safeParse(JSON.parse(storage.getItem(storageKey) ?? 'null'))
        if (saved.success && saved.data.owner === state.user.id && !state.drafts.order?.order) {
          writing = true
          store.dispatch(saveWorkDraft({ key: 'order', draft: { order: saved.data.order } }))
          writing = false
        }
      } catch { writing = false }
    }
    const order = store.getState().session.drafts.order?.order
    try {
      if (order) storage.setItem(storageKey, JSON.stringify({ owner: state.user.id, order }))
      else storage.removeItem(storageKey)
    } catch { /* The page retains the draft in memory; no credentials are persisted. */ }
  })
}
