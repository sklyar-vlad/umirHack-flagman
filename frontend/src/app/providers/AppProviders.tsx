import { useEffect, type PropsWithChildren } from 'react'
import { AuthContext } from '@/features/auth'
import { authController } from '../store/auth-runtime'
import { Provider } from 'react-redux'
import { setupListeners } from '@reduxjs/toolkit/query'
import { store } from '../store'
import { startLiveRuntime } from '../store/live-runtime'

export function AppProviders({ children }: PropsWithChildren) {
  useEffect(() => setupListeners(store.dispatch), [])
  useEffect(() => startLiveRuntime(store, authController), [])

  return <Provider store={store}><AuthContext value={authController}>{children}</AuthContext></Provider>
}
