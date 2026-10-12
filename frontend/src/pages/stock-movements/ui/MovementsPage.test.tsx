// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { movementApi, movementSchema, movementSearchError, movementSearchParams, changeMovementSearch, defaultMovementSearch, readMovementSearch } from '@/entities/stock-movement'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { AuthContext, type SessionController } from '@/features/auth'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { movementHandlers, mockMovements } from '@/app/mocks/movement-handlers'
import { inventoryBatches, mockId } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { MovementsPage } from './MovementsPage'

const server = setupServer(...movementHandlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [movementApi.reducerPath]: movementApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(800), name: 'Сотрудник', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Кладовая', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } }, middleware: (get) => get().concat(movementApi.middleware) })
}
function mount(path = '/stock-movements', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/stock-movements', element: <RoleGate section={sections.find((s) => s.id === 'movements')!}><MovementsPage /></RoleGate> }], { initialEntries: [path] })
  const auth: SessionController = { restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), hasAccess: () => true, getVersion: () => 0, registerSubscription: () => () => false }
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { router, user: userEvent.setup(), store }
}
const ready = () => screen.findByText('Найдено движений: 92. Страница 1.')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(movementApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('F26 historical stock movements', () => {
  it('accepts four historical kinds and rejects wrong signs, fractional pieces and broken order references', () => {
    for (const row of mockMovements.slice(0, 4)) expect(movementSchema.safeParse(row).success).toBe(true)
    expect(movementSchema.safeParse({ ...mockMovements[0], delta_quantity: '-1.000' }).success).toBe(false)
    expect(movementSchema.safeParse({ ...mockMovements[1], delta_quantity: '1.000' }).success).toBe(false)
    expect(movementSchema.safeParse({ ...mockMovements[1], unit: 'pcs' }).success).toBe(false)
    expect(movementSchema.safeParse({ ...mockMovements[1], order_item_id: null }).success).toBe(false)
    expect(movementSchema.safeParse({ ...mockMovements[2], delta_quantity: '-99999999999.999' }).success).toBe(true)
  })
  it('allowlists filters, preserves fractional instants and normalizes the old route after login', () => {
    const values = readMovementSearch(new URLSearchParams(`batch_id=${mockId(201)}&page=3&from=2026-10-10T08%3A00%3A00.000000001Z&token=secret&search=milk`))
    expect(movementSearchError(values)).toBeNull(); expect(movementSearchParams(values).has('token')).toBe(false); expect(movementSearchParams(values).has('search')).toBe(false)
    expect(changeMovementSearch(values, { type: 'expiration' }).page).toBe(1)
    expect(safeReturnUrl(`/movements?batch_id=${mockId(201)}&token=secret`)).toBe(`/stock-movements?batch_id=${mockId(201)}`)
    expect(movementSearchError({ ...defaultMovementSearch, batch_id: 'invalid' })).toContain('UUID')
    expect(movementSearchError({ ...defaultMovementSearch, type: 'loss' })).toContain('тип')
  })
  it('shows historical names, signed exact decimals, historical balances and the system author without reading inventory', async () => {
    mount(); await ready()
    expect(screen.getAllByText('Молоко · название при операции').length).toBeGreaterThan(0)
    expect(screen.getAllByText('-12,125 мл').length).toBeGreaterThan(0)
    expect(screen.getAllByText('987,875 мл').length).toBeGreaterThan(0)
    expect(screen.getAllByText('+1000 мл').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Система').length).toBeGreaterThan(0)
    expect(requests.every((r) => r.method === 'GET' && new URL(r.url).pathname === '/api/v1/stock-movements')).toBe(true)
  })
  it('filters, keeps URL, resets pagination and restores the same filter on back', async () => {
    const { user, router } = mount(); await ready()
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await screen.findByText('Найдено движений: 92. Страница 2.')
    await user.selectOptions(screen.getByLabelText('Тип движения'), 'expiration'); await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Найдено движений: 17. Страница 1.'); expect(router.state.location.search).toBe('?type=expiration')
    await user.click(screen.getAllByRole('button', { name: 'Движения этой партии' })[0]); await screen.findByText('Найдено движений: 6. Страница 1.')
    expect(router.state.location.search).toContain(`batch_id=${mockId(201)}`)
    await router.navigate(-1); await screen.findByText('Найдено движений: 17. Страница 1.')
  })
  it.each(['manager', 'storekeeper'] as const)('allows %s and displays order links according to that role', async (role) => {
    mount('/stock-movements', role); await ready()
    expect(screen.queryAllByRole('link', { name: 'К заказу' }).length > 0).toBe(role === 'manager')
  })
  it('blocks order operators and invalid URL filters before making any request', async () => {
    mount('/stock-movements', 'order_operator'); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it('rejects invalid IDs before making any request', async () => {
    mount('/stock-movements?batch_id=bad'); await screen.findByRole('heading', { name: 'Некорректные фильтры' }); expect(requests).toHaveLength(0)
  })
  it('resolves the supply on demand even when the historical batch is on a later API page', async () => {
    const target = inventoryBatches[0]
    server.use(http.get('/api/v1/products/:id/batches', ({ request }) => {
      const page = Number(new URL(request.url).searchParams.get('page'))
      const rows = page === 1 ? Array.from({ length: 100 }, (_, index) => ({ ...target, id: mockId(3000 + index) })) : [target]
      return Response.json({ items: rows, total: 101, page, page_size: 100, meta: {} })
    }))
    const { user } = mount(); await ready(); await user.click(screen.getAllByRole('button', { name: 'Найти поставку' })[0])
    expect((await screen.findByRole('link', { name: 'Поставка №ТЕСТ-101' })).getAttribute('href')).toBe(`/supplies/${target.supply.id}`)
    expect(requests.filter((r) => new URL(r.url).pathname.includes('/batches')).map((r) => new URL(r.url).searchParams.get('page'))).toEqual(['1', '2'])
    expect(screen.getAllByText('1000 мл').length).toBeGreaterThan(0)
  })
  it('preserves historical data after refresh failure but hides it on revoked access', async () => {
    const { user } = mount(); await ready()
    setMockScenario('movements-error'); await user.click(screen.getByRole('button', { name: 'Обновить историю' })); await screen.findByText(/Данные могут быть устаревшими/)
    expect(screen.getAllByText('Молоко · название при операции').length).toBeGreaterThan(0)
    setMockScenario('movements-forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить историю' })); await screen.findByRole('heading', { name: 'Недостаточно прав' })
    await waitFor(() => expect(screen.queryByText('Найдено движений: 92. Страница 1.')).toBeNull())
  })
  it('rejects duplicate records without caching or rendering them', async () => {
    server.use(http.get('/api/v1/stock-movements', () => Response.json({ items: [mockMovements[0], mockMovements[0]], total: 2, page: 1, page_size: 50, meta: {} })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить движения' }); expect(screen.queryByText('Найдено движений: 2. Страница 1.')).toBeNull()
  })
  it('keeps exact inclusive/exclusive period bounds in the URL after submitting unchanged local dates', async () => {
    const { user, router } = mount('/stock-movements?type=expiration&from=2026-10-10T08%3A57%3A00.000000001Z&to=2026-10-10T09%3A00%3A00Z')
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    expect(new URLSearchParams(router.state.location.search).get('from')).toBe('2026-10-10T08:57:00.000000001Z')
    expect(requests.every((r) => new URL(r.url).searchParams.get('sort') === '-occurred_at')).toBe(true)
  })
  it('keeps snapshots readable when the historical batch is unavailable today', async () => {
    server.use(http.get('/api/v1/products/:id/batches', () => Response.json({ items: [], total: 0, page: 1, page_size: 100, meta: {} })))
    const { user } = mount(); await ready(); await user.click(screen.getAllByRole('button', { name: 'Найти поставку' })[0])
    await screen.findByText('Партия недоступна. Историческая запись остаётся в журнале.')
    expect(screen.getAllByText('Молоко · название при операции').length).toBeGreaterThan(0)
  })
})
