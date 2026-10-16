import { createContext, useContext } from 'react'
import type { SessionController } from './session-controller'
export const AuthContext = createContext<SessionController | null>(null)
export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('AuthContext is required')
  return value
}
