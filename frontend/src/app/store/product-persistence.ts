import { z } from 'zod'
import { productDraftSchema } from '@/shared/api/product-work'
import { saveWorkDraft } from '@/entities/session'
import type { AppStore } from './store'

const storageKey = 'flagman.products.v1'
const storedSchema = z.object({ owner: z.uuid(), products: z.record(z.string(), productDraftSchema) })
/** Only product form data is persisted; credentials and API cache never enter storage. */
export function persistProducts(store: AppStore, storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
  let restored = false, writing = false
  return store.subscribe(() => {
    if (writing) return
    const state = store.getState().session
    if (state.status === 'signing-out') { restored = true; try { storage.removeItem(storageKey) } catch { /* Memory remains available. */ } return }
    if (state.status !== 'authenticated' || !state.user) return
    if (!restored) {
      restored = true
      try {
        const saved = storedSchema.safeParse(JSON.parse(storage.getItem(storageKey) ?? 'null'))
        if (saved.success && saved.data.owner === state.user.id) {
          writing = true
          for (const product of Object.values(saved.data.products)) {
            const key = `product:${product.resourceId}`
            if (!state.drafts[key]?.product) store.dispatch(saveWorkDraft({ key, draft: { product } }))
          }
          writing = false
        }
      } catch { writing = false }
    }
    const products = Object.fromEntries(Object.entries(store.getState().session.drafts).filter(([key, value]) => key === `product:${value.product?.resourceId}`).map(([key, value]) => [key, value.product]))
    try {
      if (Object.keys(products).length) storage.setItem(storageKey, JSON.stringify({ owner: state.user.id, products }))
      else storage.removeItem(storageKey)
    } catch { /* Storage unavailable: owner-scoped memory still retains the draft. */ }
  })
}
