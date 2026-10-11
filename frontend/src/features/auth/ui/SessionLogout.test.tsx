// @vitest-environment jsdom
import { StrictMode } from 'react'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { initialSessionState, type SessionState } from '@/entities/session'
import { AuthContext } from '../model/auth-context'
import { createSessionController } from '../model/session-controller'
import { SessionLogout } from './SessionLogout'

afterEach(cleanup)
it('sends logout once in StrictMode and clears session resources', async () => {
  let state: SessionState = { ...initialSessionState, status: 'authenticated' }
  const logout = vi.fn(async () => {})
  const clearApi = vi.fn()
  const auth = createSessionController({ logout, login: vi.fn(), me: vi.fn() }, {
    read: () => state, write: (value) => { state = value }, setCsrf: vi.fn(), clearApi,
  })
  render(<StrictMode><AuthContext value={auth}><SessionLogout /></AuthContext></StrictMode>)
  await waitFor(() => expect(state.status).toBe('anonymous'))
  expect(logout).toHaveBeenCalledTimes(1)
  expect(clearApi).toHaveBeenCalledTimes(1)
  expect(state.notice).toBeNull()
})
