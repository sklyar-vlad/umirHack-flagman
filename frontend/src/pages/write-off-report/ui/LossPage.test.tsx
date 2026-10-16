// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { lossApi, lossSchema, lossSearchError, lossSearchParams, changeLossSearch, defaultLossSearch, readLossSearch, lossMovementsUrl } from '@/entities/write-off-report'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { AuthContext, type SessionController } from '@/features/auth'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { lossHandlers } from '@/app/mocks/loss-handlers'
import { mockId } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { LossPage } from './LossPage'

const server = setupServer(...lossHandlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [lossApi.reducerPath]: lossApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(800), name: 'Сотрудник', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Кладовая', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } }, middleware: (get) => get().concat(lossApi.middleware) })
}
function mount(path = '/analytics/write-offs', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/analytics/write-offs', element: <RoleGate section={sections.find((s) => s.id === 'analytics')!}><LossPage /></RoleGate> }], { initialEntries: [path] })
  const auth: SessionController = { revalidate: vi.fn(async () => true), restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), hasAccess: () => true, getVersion: () => 0, registerSubscription: () => () => false }
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { router, user: userEvent.setup(), store }
}
const ready = () => screen.findByText('Групп продуктов и единиц: 27. Страница 1.')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(lossApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

const row = { product_id: mockId(10), product_name: 'Историческое молоко', unit: 'ml' as const, expired_written_off_quantity: '3.003', manual_written_off_quantity: '2.002', total_written_off_quantity: '5.005', movements_count: 2 }
describe('F27 loss report', () => {
  it('validates exact aggregates beyond the individual quantity bound and whole pieces', () => {
    expect(lossSchema.safeParse(row).success).toBe(true)
    expect(lossSchema.safeParse({ ...row, expired_written_off_quantity: '99999999999.999', manual_written_off_quantity: '99999999999.999', total_written_off_quantity: '199999999999.998' }).success).toBe(true)
    expect(lossSchema.safeParse({ ...row, total_written_off_quantity: '5.006' }).success).toBe(false)
    expect(lossSchema.safeParse({ ...row, unit: 'pcs' }).success).toBe(false)
    expect(lossSchema.safeParse({ ...row, expired_written_off_quantity: '-3.003' }).success).toBe(false)
  })
  it('allowlists URL fields, preserves fractional instants and resets page on filters', () => {
    const values = readLossSearch(new URLSearchParams('reason=expiration&page=3&from=2026-10-10T08%3A00%3A00.000000001Z&token=secret&sort=-created_at'))
    expect(lossSearchError(values)).toBeNull(); expect(lossSearchParams(values).has('token')).toBe(false); expect(lossSearchParams(values).has('sort')).toBe(false)
    expect(changeLossSearch(values, { product_id: mockId(10) }).page).toBe(1)
    expect(safeReturnUrl('/analytics?reason=expiration&token=secret')).toBe('/analytics/write-offs?reason=expiration')
    const target = new URL(lossMovementsUrl(row, values, 'expiration'), 'https://internal.test')
    expect(target.searchParams.get('from')).toBe(values.from); expect(target.searchParams.get('product_id')).toBe(row.product_id); expect(target.searchParams.get('type')).toBe('expiration')
    expect(lossSearchError({ ...defaultLossSearch, product_id: 'bad' })).toContain('UUID')
  })
  it('shows separate product/unit groups and exact large quantities using GET only', async () => {
    mount(); await ready()
    expect(screen.getAllByText('199999999999,998 г').length).toBeGreaterThan(0)
    expect(screen.getAllByText('3200 мл').length).toBeGreaterThan(0)
    expect(screen.getAllByText('2 шт.').length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: 'Движения просрочки' }).length).toBeGreaterThan(0)
    expect(requests.every((r) => r.method === 'GET' && new URL(r.url).pathname === '/api/v1/analytics/write-offs' && !new URL(r.url).searchParams.has('sort'))).toBe(true)
  })
  it('filters reasons, preserves periods in movement links and does not invent a unit filter', async () => {
    const { user, router } = mount(); await ready()
    await user.selectOptions(screen.getByLabelText('Причина списания'), 'expiration'); await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Групп продуктов и единиц: 2. Страница 1.'); expect(router.state.location.search).toBe('?reason=expiration')
    expect(screen.queryAllByRole('link', { name: 'Ручные движения' })).toHaveLength(0)
    const link = screen.getAllByRole('link', { name: 'Движения просрочки' })[0]
    expect(link.getAttribute('href')).toContain('type=expiration'); expect(link.getAttribute('href')).not.toContain('unit=')
  })
  it('pages with server total and restores filtering on back', async () => {
    const { user, router } = mount('/analytics/write-offs?page_size=20'); await screen.findByText('Групп продуктов и единиц: 27. Страница 1.')
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await screen.findByText('Групп продуктов и единиц: 27. Страница 2.')
    await user.type(screen.getByLabelText('ID продукта'), mockId(10)); await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Групп продуктов и единиц: 2. Страница 1.'); expect(new URLSearchParams(router.state.location.search).has('page')).toBe(false)
    await router.navigate(-1); await screen.findByText('Групп продуктов и единиц: 27. Страница 2.')
  })
  it('does not treat additional ordinary order consumption as a loss', async () => {
    const { user } = mount(); await ready(); setMockScenario('losses-order'); await user.click(screen.getByRole('button', { name: 'Обновить отчёт' })); await waitFor(() => expect(screen.getByRole('button', { name: 'Обновить отчёт' }).hasAttribute('disabled')).toBe(false))
    expect(screen.getAllByText('3200 мл').length).toBeGreaterThan(0); expect(screen.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeTruthy()
  })
  it.each(['storekeeper', 'order_operator'] as const)('blocks %s before mounting report queries', async (role) => {
    mount('/analytics/write-offs', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it('allows managers', async () => { mount('/analytics/write-offs', 'manager'); await ready() })
  it.each(['product_id=bad', 'reason=order_consumption', 'from=yesterday'])('blocks invalid URL %s', async (query) => {
    mount(`/analytics/write-offs?${query}`); await screen.findByRole('heading', { name: 'Некорректные фильтры' }); expect(requests).toHaveLength(0)
  })
  it('preserves a successful report on transient failure and hides it on 403', async () => {
    const { user } = mount(); await ready(); setMockScenario('losses-error'); await user.click(screen.getByRole('button', { name: 'Обновить отчёт' })); await screen.findByText(/Данные могут быть устаревшими/)
    expect(screen.getByText('Групп продуктов и единиц: 27. Страница 1.')).toBeTruthy()
    setMockScenario('losses-forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить отчёт' })); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByText('Групп продуктов и единиц: 27. Страница 1.')).toBeNull()
  })
  it('rejects duplicate product/unit groups while allowing the same product in different historical units', async () => {
    server.use(http.get('/api/v1/analytics/write-offs', () => Response.json({ items: [row, row], total: 2, page: 1, page_size: 50, meta: {} })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить потери' })
  })
  it('shows empty pages safely and offers the first page', async () => {
    mount('/analytics/write-offs?page=99'); await screen.findByRole('heading', { name: 'На этой странице нет потерь' }); expect(screen.getByRole('button', { name: 'К первой странице' })).toBeTruthy()
  })
  it('preserves unchanged nanosecond period boundaries when submitting local input', async () => {
    const { user, router } = mount('/analytics/write-offs?from=2026-10-10T08%3A58%3A00.000000001Z'); await screen.findByRole('heading', { name: 'Ничего не найдено' }); await user.click(screen.getByRole('button', { name: 'Найти' }))
    expect(new URLSearchParams(router.state.location.search).get('from')).toBe('2026-10-10T08:58:00.000000001Z')
  })
})
