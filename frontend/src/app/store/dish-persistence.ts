import { z } from 'zod'
import { dishDraftSchema } from '@/shared/api/dish-work'
import { saveWorkDraft } from '@/entities/session'
import type { AppStore } from './store'

const storageKey = 'flagman.dishes.v1'
const storedSchema = z.object({ owner: z.uuid(), dishes: z.record(z.string(), dishDraftSchema) })
export function persistDishes(store: AppStore, storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
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
        if (saved.success && saved.data.owner === state.user.id) {
          writing = true
          for (const dish of Object.values(saved.data.dishes)) {
            const key = `dish:${dish.resourceId}`
            if (!state.drafts[key]?.dish) store.dispatch(saveWorkDraft({ key, draft: { dish } }))
          }
          writing = false
        }
      } catch { writing = false }
    }
    const dishes = Object.fromEntries(Object.entries(store.getState().session.drafts).filter(([key, value]) => key === `dish:${value.dish?.resourceId}`).map(([key, value]) => [key, value.dish]))
    try {
      if (Object.keys(dishes).length) storage.setItem(storageKey, JSON.stringify({ owner: state.user.id, dishes }))
      else storage.removeItem(storageKey)
    } catch {  }
  })
}
