import { z } from 'zod'
import { receptionDraftSchema } from '@/shared/api'
import { saveWorkDraft } from '@/entities/session'
import type { AppStore } from './store'

const storageKey = 'flagman.reception.v1'
const storedSchema = z.object({ owner: z.uuid(), reception: receptionDraftSchema })
/** Tab-local, non-secret data only. Ownership is checked after real session restoration. */
export function persistReception(store: AppStore, storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
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
        if (saved.success && saved.data.owner === state.user.id && !state.drafts.reception?.reception) {
          writing = true
          store.dispatch(saveWorkDraft({ key: 'reception', draft: { reception: saved.data.reception } }))
          writing = false
        }
      } catch { writing = false }
    }
    const reception = store.getState().session.drafts.reception?.reception
    try {
      if (reception) storage.setItem(storageKey, JSON.stringify({ owner: state.user.id, reception }))
      else storage.removeItem(storageKey)
    } catch { /* The page retains the draft in memory; no credentials are persisted. */ }
  })
}
