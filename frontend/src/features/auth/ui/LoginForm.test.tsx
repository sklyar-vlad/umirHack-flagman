// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { initialSessionState, sessionReducer, replaceSession } from '@/entities/session'
import { AuthContext } from '../model/auth-context'
import type { SessionController } from '../model/session-controller'
import { LoginForm } from './LoginForm'

afterEach(cleanup)
function setup(login: SessionController['login'] = vi.fn(async () => {}), path = '/login') {
  const store = configureStore({ reducer: { session: sessionReducer } })
  store.dispatch(replaceSession({ ...initialSessionState, status: 'anonymous' }))
  const auth: SessionController = { revalidate: vi.fn(async () => true), login, logout: vi.fn(async () => {}), restore: vi.fn(async () => false), expire: vi.fn(), getVersion: () => 0, hasAccess: () => false, registerSubscription: () => () => false }
  const router = createMemoryRouter([{ path: '/login', element: <LoginForm /> }, { path: '/', element: <h1>Home</h1> }], { initialEntries: [path] })
  render(<Provider store={store}><AuthContext value={auth}><RouterProvider router={router} /></AuthContext></Provider>)
  return { user: userEvent.setup(), login, store, router }
}
describe('login form', () => {
  it('validates before calling the adapter and focuses the first invalid field', async () => {
    const { user, login } = setup()
    await user.click(screen.getByRole('button', { name: 'Войти' }))
    expect(screen.getByText('Введите корректный email.')).toBeTruthy()
    expect(screen.getByText('Введите пароль.')).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByLabelText(/Email/))
    expect(login).not.toHaveBeenCalled()
  })
  it('shows/hides password by keyboard and clears it after a failed attempt', async () => {
    const login = vi.fn(async () => { throw { status: 401, code: 'UNAUTHORIZED', message: 'untrusted-secret', details: {} } })
    const { user } = setup(login)
    await user.type(screen.getByLabelText(/Email/), 'employee@example.test')
    const password = screen.getByLabelText(/^Пароль/)
    await user.type(password, 'test-secret')
    const toggle = screen.getByRole('button', { name: 'Показать пароль' })
    toggle.focus()
    await user.keyboard(' ')
    expect(password.getAttribute('type')).toBe('text')
    expect(screen.getByRole('button', { name: 'Скрыть пароль' }).getAttribute('aria-pressed')).toBe('true')
    await user.click(screen.getByRole('button', { name: 'Войти' }))
    await screen.findByText('Неверный email или пароль.')
    expect(document.activeElement?.getAttribute('role')).toBe('alert')
    expect((password as HTMLInputElement).value).toBe('')
    expect(password.getAttribute('type')).toBe('password')
    expect(screen.queryByText('untrusted-secret')).toBeNull()
  })
  it('disables all fields and avoids repeated submission while pending', async () => {
    let finish!: () => void
    const login = vi.fn(() => new Promise<void>((resolve) => { finish = resolve }))
    const { user } = setup(login)
    await user.type(screen.getByLabelText(/Email/), 'employee@example.test')
    await user.type(screen.getByLabelText(/^Пароль/), 'test-secret')
    await user.click(screen.getByRole('button', { name: 'Войти' }))
    expect((screen.getByLabelText(/Email/) as HTMLInputElement).disabled).toBe(true)
    expect((screen.getByLabelText(/^Пароль/) as HTMLInputElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'Входим…' }) as HTMLButtonElement).disabled).toBe(true)
    await user.keyboard('{Enter}')
    expect(login).toHaveBeenCalledTimes(1)
    finish()
    await waitFor(() => expect((screen.getByRole('button', { name: 'Войти' }) as HTMLButtonElement).disabled).toBe(false))
    expect((screen.getByLabelText(/^Пароль/) as HTMLInputElement).value).toBe('')
  })
  it('locks retries for Retry-After and never stores credentials in Redux', async () => {
    const { user, store } = setup(vi.fn(async () => { throw { status: 429, code: 'RATE_LIMIT_EXCEEDED', message: '', details: {}, retry_after: '1' } }))
    await user.type(screen.getByLabelText(/Email/), 'employee@example.test')
    await user.type(screen.getByLabelText(/^Пароль/), 'test-secret')
    await user.click(screen.getByRole('button', { name: 'Войти' }))
    await screen.findByText('Повторите вход через 1 сек.')
    expect((screen.getByRole('button', { name: 'Войти' }) as HTMLButtonElement).disabled).toBe(true)
    expect(JSON.stringify(store.getState())).not.toContain('test-secret')
    await waitFor(() => expect((screen.getByRole('button', { name: 'Войти' }) as HTMLButtonElement).disabled).toBe(false), { timeout: 2000 })
  })
})
