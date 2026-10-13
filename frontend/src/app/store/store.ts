import { dishChangesReducer } from '@/entities/dish'
import { configureStore } from '@reduxjs/toolkit'
import { sessionReducer } from '@/entities/session'
import { baseApi } from '@/shared/api'
import { apiTransport } from './session-transport'
import { liveMutations } from './live-mutations'

export const store = configureStore({
  reducer: {
    session: sessionReducer,
    dishChanges: dishChangesReducer,
    [baseApi.reducerPath]: baseApi.reducer,
  },
  middleware: (getDefaultMiddleware) => getDefaultMiddleware({
    thunk: { extraArgument: apiTransport },
  }).prepend(liveMutations.middleware).concat(baseApi.middleware),
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
export type AppStore = typeof store
