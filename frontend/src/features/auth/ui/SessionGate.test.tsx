// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, act } from '@testing-library/react'
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { initialSessionState, sessionReducer, replaceSession } from '@/entities/session'
import { createSessionController } from '../model/session-controller'
import { AuthContext } from '../model/auth-context'
import { SessionGate } from './SessionGate'

afterEach(() => { cleanup(); vi.useRealTimers() })
describe('session route boundary', () => {
  it('hides protected content at expiry and preserves only the safe internal return path', async () => {
    vi.useFakeTimers()
    const store = configureStore({ reducer: { session: sessionReducer } })
    store.dispatch(replaceSession({ ...initialSessionState, status: 'authenticated', expiresAt: new Date(Date.now() + 1000).toISOString(), user: { id: 'test-user', name: 'Тест', email: 'test@example.test', role: 'manager' } }))
    const auth = createSessionController({ me: vi.fn(), login: vi.fn(), logout: vi.fn() }, {
      read: () => store.getState().session, write: (state) => { store.dispatch(replaceSession(state)) }, setCsrf: vi.fn(), clearApi: vi.fn(),
    })
    const router = createMemoryRouter([
      { Component: SessionGate, children: [{ path: '/private', element: <h1>Protected content</h1> }] },
      { path: '/login', element: <h1>Login</h1> },
    ], { initialEntries: ['/private?password=secret#token'] })
    render(<Provider store={store}><AuthContext value={auth}><RouterProvider router={router} /></AuthContext></Provider>)
    expect(screen.getByText('Protected content')).toBeTruthy()
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(screen.queryByText('Protected content')).toBeNull()
    expect(screen.getByText('Login')).toBeTruthy()
    expect(router.state.location.pathname + router.state.location.search).toBe('/login?returnTo=%2Fprivate')
    expect(store.getState().session.user).toBeNull()
  })
})
