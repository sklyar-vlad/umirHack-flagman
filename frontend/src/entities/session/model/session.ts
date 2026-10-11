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
export interface WorkDraft { title?: string; note?: string }
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
    replaceSession: (_state, action: PayloadAction<SessionState>) => action.payload,
    saveWorkDraft: (state, { payload }: PayloadAction<{ key: string; draft: WorkDraft }>) => {
      if (state.status !== 'authenticated' || !state.user) return
      state.draftOwner = state.user.id
      // Explicit allowlist also strips unexpected keys supplied by JS callers.
      state.drafts[payload.key] = {
        ...(typeof payload.draft.title === 'string' ? { title: payload.draft.title } : {}),
        ...(typeof payload.draft.note === 'string' ? { note: payload.draft.note } : {}),
      }
    },
    replaceBasket: (state, { payload }: PayloadAction<BasketLine[]>) => {
      if (state.status === 'authenticated') state.basket = payload.map(({ dishId, portions }) => ({ dishId, portions }))
    },
  },
})
export const sessionReducer = slice.reducer
export const { replaceSession, saveWorkDraft, replaceBasket } = slice.actions
