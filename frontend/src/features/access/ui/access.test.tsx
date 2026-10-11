// @vitest-environment jsdom
import { useEffect } from 'react'
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { MemoryRouter } from 'react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { initialSessionState, sections, sessionReducer, type Role, type SessionState } from '@/entities/session'
import { RoleGate } from './RoleGate'
import { ApiAccessBoundary } from './ApiAccessBoundary'

afterEach(cleanup)
function session(role: Role): SessionState {
  return { ...initialSessionState, status: 'authenticated', user: { id: 'test', name: 'Тест', email: 'test@example.test', role } }
}
function guarded(state: SessionState, onMount: () => void) {
  const store = configureStore({ reducer: { session: sessionReducer }, preloadedState: { session: state } })
  function BusinessPage() { useEffect(onMount, []); return <p>Данные заказов</p> }
  render(<Provider store={store}><MemoryRouter><RoleGate section={sections.find((item) => item.id === 'orders')!}><BusinessPage /></RoleGate></MemoryRouter></Provider>)
}
describe('route access', () => {
  it('does not mount business queries before permissions are known', () => {
    const mount = vi.fn()
    guarded(initialSessionState, mount)
    expect(screen.getByRole('status').getAttribute('aria-label')).toBe('Проверяем права')
    expect(mount).not.toHaveBeenCalled()
  })
  it('does not mount a forbidden screen and shows a safe return link', () => {
    const mount = vi.fn()
    guarded(session('storekeeper'), mount)
    expect(screen.getByRole('heading', { name: 'Недостаточно прав' })).toBeTruthy()
    expect(screen.getByRole('link').getAttribute('href')).toBe('/')
    expect(mount).not.toHaveBeenCalled()
    expect(screen.queryByText('Данные заказов')).toBeNull()
  })
  it('mounts an allowed screen', () => {
    const mount = vi.fn()
    guarded(session('order_operator'), mount)
    expect(mount).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Данные заказов')).toBeTruthy()
  })
  it('handles server 403 without disclosing its details and restores output after recovery', () => {
    const view = render(<MemoryRouter><ApiAccessBoundary error={{ status: 403, code: 'PRIVATE_POLICY', message: 'private-server-message', details: { secret: 'internal' } }}><p>Частные данные</p></ApiAccessBoundary></MemoryRouter>)
    expect(screen.getByRole('heading', { name: 'Недостаточно прав' })).toBeTruthy()
    expect(screen.queryByText('Частные данные')).toBeNull()
    expect(document.body.textContent).not.toContain('private-server-message')
    expect(document.body.textContent).not.toContain('internal')
    view.rerender(<MemoryRouter><ApiAccessBoundary error={undefined}><p>Частные данные</p></ApiAccessBoundary></MemoryRouter>)
    expect(screen.getByText('Частные данные')).toBeTruthy()
  })
  it('leaves other error states to the consuming page', () => {
    render(<MemoryRouter><ApiAccessBoundary error={{ status: 500, code: 'INTERNAL_ERROR', message: 'Error', details: {} }}><p>Повторить запрос</p></ApiAccessBoundary></MemoryRouter>)
    expect(screen.getByText('Повторить запрос')).toBeTruthy()
  })
})
