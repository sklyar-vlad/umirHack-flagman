// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { auditApi, auditEntrySchema, auditSearchError, auditSearchParams, changeAuditSearch, defaultAuditSearch, readAuditSearch } from '@/entities/audit-log'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { AuthContext, type SessionController } from '@/features/auth'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { auditHandlers, mockAuditEntries } from '@/app/mocks/audit-handlers'
import { setMockScenario } from '@/app/mocks/scenarios'
import { AuditPage } from './AuditPage'

const server = setupServer(...auditHandlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [auditApi.reducerPath]: auditApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockAuditEntries[0].actor!.id, name: 'Администратор', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Кладовая', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } }, middleware: (get) => get().concat(auditApi.middleware) })
}
function mount(path = '/admin/audit-log', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/admin/audit-log', element: <RoleGate section={sections.find((s) => s.id === 'audit')!}><AuditPage /></RoleGate> }], { initialEntries: [path] })
  const auth: SessionController = { revalidate: vi.fn(async () => true), restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), hasAccess: () => true, getVersion: () => 0, registerSubscription: () => () => false }
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { router, user: userEvent.setup(), store }
}
const ready = () => screen.findByText('Найдено записей: 65. Страница 1.')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }; HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(auditApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('F24 audit journal', () => {
  it('removes unknown secrets from every typed snapshot and rejects invalid envelopes', () => {
    const value = auditEntrySchema.parse({ ...mockAuditEntries[0], before: { name: '<script>alert(1)</script>', password_hash: 'hidden' }, after: { name: 'Молоко', token: 'hidden' } })
    expect(value.before).toEqual({ name: '<script>alert(1)</script>' }); expect(value.after).toEqual({ name: 'Молоко' })
    const nested = auditEntrySchema.parse({ ...mockAuditEntries[0], entity_type: 'dish', before: null, after: { recipe: [{ product_id: 'id', quantity_per_portion: '1.000', password: 'hidden' }] } })
    expect(JSON.stringify(nested)).not.toContain('hidden')
    expect(auditEntrySchema.safeParse({ ...mockAuditEntries[0], created_at: 'yesterday' }).success).toBe(false)
  })
  it('allowlists URL parameters, preserves fractions and resets page after changing a filter', () => {
    const values = readAuditSearch(new URLSearchParams('actor_id=system&action=expiration&page=3&from=2026-10-10T08%3A00%3A00.000000001Z&token=secret'))
    expect(auditSearchError(values)).toBeNull(); expect(auditSearchParams(values).has('token')).toBe(false)
    expect(changeAuditSearch(values, { action: '' }).page).toBe(1)
    expect(auditSearchError({ ...defaultAuditSearch, entity_id: 'invalid' })).toContain('UUID')
    expect(safeReturnUrl('/admin/audit?actor_id=system&token=secret')).toBe('/admin/audit-log?actor_id=system')
  })
  it('reads GET only, filters system expiration, keeps URL and shows before/after', async () => {
    const { user, router } = mount(); await ready()
    await user.click(screen.getByText('Фильтры по автору, объекту и периоду'))
    await user.type(screen.getByLabelText('Автор (UUID или system)'), 'system')
    await user.selectOptions(screen.getByLabelText('Действие'), 'expiration')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Найдено записей: 1. Страница 1.')
    expect(router.state.location.search).toContain('actor_id=system')
    await user.click(screen.getAllByRole('button', { name: `Изменения записи ${mockAuditEntries[1].id}` })[0])
    const dialog = await screen.findByRole('dialog', { name: 'Изменения записи' })
    expect(within(dialog).getByText('Система')).toBeTruthy()
    expect(within(dialog).getByText('1000.000')).toBeTruthy(); expect(within(dialog).getByText('0.000')).toBeTruthy()
    expect(within(dialog).getByText(mockAuditEntries[1].request_id!)).toBeTruthy()
    expect(requests.every((request) => request.method === 'GET' && new URL(request.url).pathname === '/api/v1/audit-log')).toBe(true)
  })
  it('clears an unsubmitted draft on reset and pages using server total', async () => {
    const { user, router } = mount(); await ready()
    await user.type(screen.getByLabelText('Поиск по действию, пользователю или объекту'), 'pending')
    await user.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
    expect((screen.getByLabelText('Поиск по действию, пользователю или объекту') as HTMLInputElement).value).toBe('')
    await user.click(screen.getByRole('button', { name: 'Страница 2' }))
    await screen.findByText('Найдено записей: 65. Страница 2.'); expect(router.state.location.search).toBe('?page=2')
  })
  it('does not send invalid URL filters to the backend', async () => {
    mount('/admin/audit-log?actor_id=bad'); await screen.findByRole('heading', { name: 'Некорректные фильтры' }); expect(requests).toHaveLength(0)
  })
  it.each(['manager', 'storekeeper', 'order_operator'] as const)('never queries the journal for %s', async (role) => {
    mount('/admin/audit-log', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it('escapes displayed fields and strips secret content before caching', async () => {
    server.use(http.get('/api/v1/audit-log', () => Response.json({ items: [{ ...mockAuditEntries[0], before: { name: '<img src=x onerror=alert(1)>', password: 'hidden' } }], total: 1, page: 1, page_size: 50, meta: {} })))
    const { user, store } = mount(); await screen.findByText('Найдено записей: 1. Страница 1.')
    await user.click(screen.getAllByRole('button', { name: `Изменения записи ${mockAuditEntries[0].id}` })[0])
    const dialog = await screen.findByRole('dialog'); expect(within(dialog).getByText('<img src=x onerror=alert(1)>')).toBeTruthy()
    expect(dialog.querySelector('img[src="x"]')).toBeNull(); expect(JSON.stringify(store.getState().api.queries)).not.toContain('hidden')
  })
  it('preserves same-filter data after failure but hides list and open details on 403', async () => {
    const { user, store } = mount(); await ready()
    setMockScenario('audit-error'); await user.click(screen.getByRole('button', { name: 'Обновить журнал' }))
    await screen.findByText(/Данные могут быть устаревшими/); expect(screen.getByText('Найдено записей: 65. Страница 1.')).toBeTruthy()
    await user.click(screen.getAllByRole('button', { name: `Изменения записи ${mockAuditEntries[0].id}` })[0])
    setMockScenario('audit-forbidden'); store.dispatch(auditApi.util.invalidateTags(['AuditLog']))
    await screen.findByRole('heading', { name: 'Недостаточно прав' })
    await waitFor(() => expect(screen.queryByText('Найдено записей: 65. Страница 1.')).toBeNull())
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
