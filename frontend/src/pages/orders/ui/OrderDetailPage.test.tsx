// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { initialSessionState, sessionReducer, type Role } from '@/entities/session'
import { invalidateOrders, orderApi, orderDetailSchema } from '@/entities/order'
import { orderHandlers } from '@/app/mocks/order-handlers'
import { orderDetailFixture as fixture, splitOrderDetailFixture } from '@/app/mocks/order-detail-fixtures'
import { orderDemoSupplies, orderDemoInventory } from '@/app/mocks/order-detail-warehouse-fixtures'
import { inventoryResponseSchema } from '@/entities/inventory'
import { supplyDetailSchema } from '@/entities/supply'
import { inventoryServerTime } from '@/app/mocks/inventory-fixtures'
import { acceptOrder, compositionCheck, compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { OrderDetailPage } from './OrderDetailPage'
import { OrdersPage } from './OrdersPage'

const server = setupServer(...orderHandlers), NativeRequest = globalThis.Request, requests: URL[] = []
function makeStore(role: Role) {
  return configureStore({
    reducer: { session: sessionReducer, [orderApi.reducerPath]: orderApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: fixture.created_by.id, name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(orderApi.middleware),
  })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = `/orders/${fixture.id}`, role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/orders/:id', element: <OrderDetailPage /> }, { path: '/orders', element: <OrdersPage /> }], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const table = () => screen.findByRole('table', { name: 'Фактические списания: Пицца «Маргарита»' })
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(orderApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('immutable OrderDetail in explicit MSW mode', () => {
  it.each(['order-detail-demo', 'order-detail-split'] as const)('keeps %s warehouse DTOs valid and opt-in', (scenario) => {
    expect(orderDemoSupplies()).toEqual([]); expect(orderDemoInventory()).toEqual([])
    setMockScenario(scenario)
    const supplies = orderDemoSupplies(), items = orderDemoInventory()
    expect(supplies.every((supply) => supplyDetailSchema.safeParse(supply).success)).toBe(true)
    expect(inventoryResponseSchema.safeParse({ items, page: 1, page_size: 50, total: 3, meta: { server_time: inventoryServerTime }, counts: { all: 3, expired: 0, expiring: 0, out_of_stock: 0, deficit: 0 }, attention_batches: [], attention_total: 0 }).success).toBe(true)
    const batches = supplies.flatMap((supply) => supply.items.map((line) => line.batch))
    expect(batches).toHaveLength(scenario === 'order-detail-split' ? 4 : 3)
    expect(batches[0].id).toBe(fixture.items[0].allocations[0].batch_id)
    expect(batches[0].remaining_quantity).toBe(scenario === 'order-detail-split' ? '0.000' : '960.000')
    expect(items.find((item) => item.product.id === batches[0].product_id)?.available_quantity).toBe(scenario === 'order-detail-split' ? '840.001' : '960.000')
  })
  it('shows all published ingredients/identities, 240 g, restaurant date and recipe without dish reads', async () => {
    mount(); const rows = within(await table()).getAllByRole('row').slice(1)
    expect(orderDetailSchema.safeParse(fixture).success).toBe(true)
    expect(screen.getByRole('heading', { name: 'Заказ №247' })).toBeTruthy()
    expect(screen.getByText('09.10.2026, 12:00:00')).toBeTruthy()
    expect(screen.getByText('Оператор')).toBeTruthy(); expect(screen.getByText('Рецептура: 1')).toBeTruthy()
    expect(rows).toHaveLength(3)
    for (const [i, allocation] of fixture.items[0].allocations.entries()) {
      for (const text of [allocation.product_name, allocation.batch_id, allocation.supply_id, `Поставка 104, строка ${i + 1}`]) expect(rows[i].textContent).toContain(text)
    }
    expect(rows[0].textContent).toContain('240 г'); expect(rows[0].textContent).toContain('960 г')
    expect(rows[1].textContent).toContain('200 г'); expect(rows[2].textContent).toContain('2 шт.')
    expect(requests.map((r) => r.pathname)).toEqual([`/api/v1/orders/${fixture.id}`])
  })
  it('keeps multiple batches and fractional quantities in server order', async () => {
    setMockScenario('order-detail-split'); mount(); const rows = within(await table()).getAllByRole('row').slice(1)
    expect(rows).toHaveLength(4)
    expect(rows[0].textContent).toContain('120,001 г'); expect(rows[0].textContent).toContain('0 г')
    expect(rows[1].textContent).toContain('119,999 г'); expect(rows[1].textContent).toContain('840,001 г')
    expect(rows[1].textContent).toContain(splitOrderDetailFixture.items[0].allocations[1].batch_id)
    expect(rows[2].textContent).toContain('Томаты'); expect(rows[3].textContent).toContain('Тесто для пиццы')
  })
  it('renders each position with its own recipe and allocations', async () => {
    server.use(http.get('/api/v1/orders/:id', () => Response.json({ ...fixture, items: [...fixture.items, { ...fixture.items[0], id: '99999999-9999-4999-8999-999999999999', dish_name: 'Снимок второго блюда', recipe_version: 3 }] })))
    mount(); await table()
    expect(screen.getByRole('heading', { name: 'Снимок второго блюда — 2 порц.' })).toBeTruthy()
    expect(screen.getByText('Рецептура: 3')).toBeTruthy(); expect(screen.getAllByRole('table')).toHaveLength(2)
  })
  it.each(['admin', 'manager', 'order_operator'] as const)('uses exact warehouse selection and permissions for %s', async (role) => {
    mount(undefined, role); const link = within(await table()).getAllByRole('link')[0]
    const url = new URL(link.getAttribute('href')!, 'https://internal.invalid')
    expect(url.pathname).toBe('/inventory'); expect(url.searchParams.get('product_id')).toBe(fixture.items[0].allocations[0].product_id)
    expect(url.searchParams.get('search')).toBe('Моцарелла')
    expect(url.searchParams.get('supply_id')).toBe(role === 'order_operator' ? null : fixture.items[0].allocations[0].supply_id)
  })
  it('reads an F10-created order from the same synthetic state', async () => {
    const body = { items: [{ dish_id: compositionDishes()[0].id, portions: 2, recipe_version: 1 }] }
    const order = acceptOrder(crypto.randomUUID(), body, compositionCheck(body))
    mount(`/orders/${order.id}`); await screen.findByRole('heading', { name: `Заказ №${order.number}` })
    expect(screen.getByRole('table').textContent).toContain(order.items[0].allocations[0].batch_id)
  })
  it('restores filters of both history tabs through keyboard return', async () => {
    setMockScenario('order-detail-demo')
    const history = '/orders?orders_search=247&orders_page_size=20&attempts_page=2&attempts_reason_code=RECIPE_CHANGED'
    const { user, router } = mount(`/orders/${fixture.id}?returnTo=${encodeURIComponent(history)}`)
    await table(); screen.getByRole('link', { name: 'К истории заказов' }).focus(); await user.keyboard('{Enter}')
    expect(await screen.findAllByRole('link', { name: 'Заказ 247' })).toHaveLength(2)
    expect(router.state.location.search).toContain('attempts_page=2'); expect(router.state.location.search).toContain('attempts_reason_code=RECIPE_CHANGED')
  })
  it('validates IDs without a request and sanitizes external history return', async () => {
    mount('/orders/wrong?returnTo=https://evil.test'); await screen.findByRole('heading', { name: 'Некорректный адрес заказа' })
    expect(requests).toHaveLength(0); expect(screen.getByRole('link', { name: 'К истории заказов' }).getAttribute('href')).toBe('/orders')
  })
  it('accepts a canonical server UUID for the same uppercase resource address', async () => {
    const id = 'abcdefab-cdef-4abc-8abc-abcdefabcdef'
    server.use(http.get('/api/v1/orders/:id', () => Response.json({ ...fixture, id })))
    mount(`/orders/${id.toUpperCase()}`); await table()
    expect(screen.getByRole('heading', { name: 'Заказ №247' })).toBeTruthy()
  })
  it('suppresses a cached order on resource 404', async () => {
    const { store } = mount(); await table(); setMockScenario('order-not-found'); store.dispatch(invalidateOrders())
    await screen.findByRole('heading', { name: 'Заказ не найден' })
    expect(screen.queryByRole('heading', { name: 'Заказ №247' })).toBeNull(); expect(screen.queryByRole('table')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Попробовать снова' })).toBeNull()
  })
  it('handles first-load error, keyboard retry, stale snapshot and 403 without logout', async () => {
    setMockScenario('server-error'); const { user, store } = mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить заказ' }); expect(screen.queryByRole('table')).toBeNull()
    setMockScenario('success'); screen.getByRole('button', { name: 'Попробовать снова' }).focus(); await user.keyboard('{Enter}'); await table()
    setMockScenario('network-error'); store.dispatch(invalidateOrders())
    await screen.findByText('Показан последний успешный снимок этого заказа. Данные могут быть устаревшими.'); expect(screen.getByRole('table')).toBeTruthy()
    setMockScenario('forbidden'); store.dispatch(invalidateOrders()); await screen.findByRole('heading', { name: 'Недостаточно прав' })
    expect(screen.queryByRole('table')).toBeNull(); expect(store.getState().session.status).toBe('authenticated')
  })
  it('never renders the previous ID while a different resource loads', async () => {
    const { router } = mount(); await table(); setMockScenario('slow'); await router.navigate('/orders/99999999-9999-4999-8999-999999999999')
    await screen.findByRole('status', { name: 'Загрузка заказа' }); expect(screen.queryByRole('heading', { name: 'Заказ №247' })).toBeNull()
    await screen.findByRole('heading', { name: 'Заказ не найден' }, { timeout: 4000 })
  })
  it('ignores a delayed prior-ID response after navigation', async () => {
    let release!: () => void
    const pending = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get(`/api/v1/orders/${fixture.id}`, async () => { await pending; return Response.json(fixture) }))
    const { router, store } = mount(); await waitFor(() => expect(requests).toHaveLength(1))
    await router.navigate('/orders/99999999-9999-4999-8999-999999999999'); await screen.findByRole('heading', { name: 'Заказ не найден' })
    release(); await waitFor(() => expect(orderApi.endpoints.order.select(fixture.id)(store.getState()).status).toBe('fulfilled'))
    expect(screen.queryByRole('table')).toBeNull()
  })
  it.each([
    { id: '99999999-9999-4999-8999-999999999999' }, { items: [] }, { items: [{ ...fixture.items[0], recipe_version: null }] },
    { items: [{ ...fixture.items[0], allocations: [{ ...fixture.items[0].allocations[0], quantity: '0.000' }] }] },
    { items: [{ ...fixture.items[0], allocations: [{ ...fixture.items[0].allocations[2], quantity: '0.001' }] }] },
  ])('rejects wrong ID or malformed published contract: %j', async (patch) => {
    server.use(http.get('/api/v1/orders/:id', () => Response.json({ ...fixture, ...patch })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить заказ' }); expect(screen.queryByRole('table')).toBeNull()
  })
})
