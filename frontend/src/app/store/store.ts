import { configureStore } from '@reduxjs/toolkit'
import { sessionReducer } from '@/entities/session'
import { baseApi } from '@/shared/api'
import { apiTransport } from './session-transport'

export const store = configureStore({
  reducer: {
    session: sessionReducer,
    [baseApi.reducerPath]: baseApi.reducer,
  },
  middleware: (getDefaultMiddleware) => getDefaultMiddleware({
    thunk: { extraArgument: apiTransport },
  }).concat(baseApi.middleware),
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
export type AppStore = typeof store
