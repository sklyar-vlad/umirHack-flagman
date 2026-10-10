import { useEffect, type PropsWithChildren } from 'react'
import { Provider } from 'react-redux'
import { setupListeners } from '@reduxjs/toolkit/query'
import { store } from '../store'

export function AppProviders({ children }: PropsWithChildren) {
  useEffect(() => setupListeners(store.dispatch), [])

  return <Provider store={store}>{children}</Provider>
}
