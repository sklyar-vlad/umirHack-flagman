import { writeOffDraftSchema, type WriteOffDraft } from '@/shared/api/write-off-work'
import { dishDraftSchema, type DishDraft } from '@/shared/api/dish-work'
import { productDraftSchema, type ProductDraft } from '@/shared/api/product-work'
import { orderDraftSchema, type OrderDraft } from '@/shared/api/order-work'
import { receptionDraftSchema, type ReceptionDraft } from '@/shared/api/supply-request'
import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

export interface SessionUser {
  id: string
  name: string
  email: string
  role: 'admin' | 'manager' | 'storekeeper' | 'order_operator'
}
export interface SessionSettings { timezone: string; restaurantName: string; expiringThresholdHours: number; autoWriteoffExpired: boolean; revision: number }
export interface SessionSnapshot { user: SessionUser; settings: SessionSettings; csrfToken: string; expiresAt: string }
/** Only these non-secret fields may survive expiration; never store arbitrary form values. */
export interface WorkDraft { title?: string; note?: string; reception?: ReceptionDraft; order?: OrderDraft; product?: ProductDraft; dish?: DishDraft; writeOff?: WriteOffDraft }
export interface BasketLine { dishId: string; portions: number }
export interface SessionState {
  status: 'unknown' | 'restoring' | 'authenticated' | 'anonymous' | 'error' | 'signing-out'
  user: SessionUser | null
  settings: SessionSettings | null
  expiresAt: string | null
  draftOwner: string | null
  drafts: Record<string, WorkDraft>
  basket: BasketLine[]
  notice: string | null
}
export const initialSessionState: SessionState = {
  status: 'unknown', user: null, settings: null, expiresAt: null, draftOwner: null, drafts: {}, basket: [], notice: null,
}
const slice = createSlice({
  name: 'session',
  initialState: initialSessionState,
  reducers: {
    updateSessionSettings: (state, { payload }: PayloadAction<SessionSettings>) => {
      if (state.status === 'authenticated' && (!state.settings || payload.revision >= state.settings.revision)) state.settings = payload
    },
    replaceSession: (_state, action: PayloadAction<SessionState>) => {
      const next = action.payload
      const order = next.status === 'authenticated' && next.draftOwner === next.user?.id ? next.drafts.order?.order : undefined
      return order ? { ...next, basket: order.lines.filter((line) => /^\d+$/.test(line.portions) && Number(line.portions) >= 1 && Number(line.portions) <= 1000).map((line) => ({ dishId: line.dish.id, portions: Number(line.portions) })) } : next
    },
    saveWorkDraft: (state, { payload }: PayloadAction<{ key: string; draft: WorkDraft }>) => {
      if (state.status !== 'authenticated' || !state.user) return
      state.draftOwner = state.user.id
      // Explicit allowlist also strips unexpected keys supplied by JS callers.
      const writeOff = writeOffDraftSchema.safeParse(payload.draft.writeOff)
      const reception = receptionDraftSchema.safeParse(payload.draft.reception)
      const order = orderDraftSchema.safeParse(payload.draft.order)
      const dish = dishDraftSchema.safeParse(payload.draft.dish)
      const product = productDraftSchema.safeParse(payload.draft.product)
      if (payload.key === 'order' && order.success) state.basket = order.data.lines.filter((line) => /^\d+$/.test(line.portions) && Number(line.portions) >= 1 && Number(line.portions) <= 1000).map((line) => ({ dishId: line.dish.id, portions: Number(line.portions) }))
      state.drafts[payload.key] = {
        ...(writeOff.success ? { writeOff: writeOff.data } : {}),
        ...(reception.success ? { reception: reception.data } : {}),
        ...(order.success ? { order: order.data } : {}),
        ...(dish.success ? { dish: dish.data } : {}),
        ...(product.success ? { product: product.data } : {}),
        ...(typeof payload.draft.title === 'string' ? { title: payload.draft.title } : {}),
        ...(typeof payload.draft.note === 'string' ? { note: payload.draft.note } : {}),
      }
    },
    clearWorkDraft: (state, { payload }: PayloadAction<string>) => { delete state.drafts[payload]; if (payload === 'order') state.basket = [] },
    replaceBasket: (state, { payload }: PayloadAction<BasketLine[]>) => {
      if (state.status === 'authenticated') state.basket = payload.map(({ dishId, portions }) => ({ dishId, portions }))
    },
  },
})
export const sessionReducer = slice.reducer
export const { replaceSession, saveWorkDraft, clearWorkDraft, replaceBasket, updateSessionSettings } = slice.actions
export const selectSession = (state: { session: SessionState }) => state.session
