import { z } from 'zod'
import { writeOffDraftSchema } from '@/shared/api/write-off-work'
import { saveWorkDraft } from '@/entities/session'
import type { AppStore } from './store'

const storageKey = 'flagman.write-off.v1'
const storedSchema = z.object({ owner: z.uuid(), writeOff: writeOffDraftSchema })
export function persistWriteOff(store: AppStore, storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
  let restored = false, writing = false
  return store.subscribe(() => {
    if (writing) return
    const state = store.getState().session
    if (state.status === 'signing-out') { restored = true; try { storage.removeItem(storageKey) } catch {  } return }
    if (state.status !== 'authenticated' || !state.user) return
    if (!restored) {
      restored = true
      try {
        const saved = storedSchema.safeParse(JSON.parse(storage.getItem(storageKey) ?? 'null'))
        if (saved.success && saved.data.owner === state.user.id && !state.drafts.writeOff?.writeOff) {
          writing = true
          store.dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: saved.data.writeOff } }))
          writing = false
        }
      } catch { writing = false }
    }
    const writeOff = store.getState().session.drafts.writeOff?.writeOff
    try {
      if (writeOff) storage.setItem(storageKey, JSON.stringify({ owner: state.user.id, writeOff }))
      else storage.removeItem(storageKey)
    } catch {  }
  })
}
