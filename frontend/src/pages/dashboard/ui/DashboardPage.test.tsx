// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { delay } from 'msw'
import { http } from 'msw/http'
import { dashboardApi, dashboardSchema } from '@/entities/dashboard'
import { initialSessionState, sessionReducer, permittedReturnPath, type Role } from '@/entities/session'
import { orderApi } from '@/entities/order'
import { supplyApi } from '@/entities/supply'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { dashboardHandlers } from '@/app/mocks/dashboard-handlers'
import { dashboardSnapshot } from '@/app/mocks/dashboard-fixtures'
import { inventoryItems } from '@/app/mocks/inventory-fixtures'
import { resetCompositionFixtures, compositionDishes } from '@/app/mocks/order-composition-fixtures'
import { resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { receptionHandlers } from '@/app/mocks/reception-handlers'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { setMockScenario } from '@/app/mocks/scenarios'
import { DashboardPage } from './DashboardPage'

const server = setupServer(...dashboardHandlers, ...receptionHandlers, ...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: URL[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [dashboardApi.reducerPath]: dashboardApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'UTC', restaurantName: 'Старое имя', expiringThresholdHours: 1, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(dashboardApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(role: Role = 'manager', path = '/dashboard') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/', element: <DashboardPage /> }, { path: '/dashboard', element: <DashboardPage /> }, { path: '*', element: <p>Назначение</p> }], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const ready = () => screen.findByRole('heading', { name: 'Требуют внимания' })
const metric = (name: string) => within(screen.getByRole('region', { name })).getByText(/^\d+$/).textContent
const readCount = () => requests.filter((url) => url.pathname === '/api/v1/dashboard').length
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); resetReceptionFixtures(); requests.length = 0; document.cookie = 'flagman_mock_role=; Max-Age=0; Path=/' })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dashboardApi.util.resetApiState())); server.resetHandlers(); vi.useRealTimers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('published dashboard in explicit MSW', () => {
  it('loads a single coherent DTO without query parameters or detail reads', async () => {
    mount('manager', '/dashboard?search=secret&page=9'); await ready()
    expect(requests.map((url) => url.pathname + url.search)).toEqual(['/api/v1/dashboard'])
    const snapshot = dashboardSnapshot()
    expect(metric('Истекающие партии')).toBe(String(snapshot.expiring_batches_count))
    expect(metric('Просроченные партии')).toBe(String(snapshot.expired_batches_count))
    expect(metric('Блюда в стоп-листе')).toBe(String(snapshot.stop_list_dishes_count))
    expect(metric('Продукты без годного остатка')).toBe(String(snapshot.out_of_stock_products_count))
    expect(screen.getByText(/Всего проблемных партий: 3. Показано: 3/)).toBeTruthy()
    expect(screen.getByRole('article', { name: 'Партия: Молоко, строка 2' }).textContent).toContain('500 мл')
  })
  it('uses snapshot settings/time rather than stale session or client counters', async () => {
    const snapshot = dashboardSnapshot(); snapshot.meta.server_time = '2001-01-02T03:04:00Z'; snapshot.settings = { restaurant_name: 'Другой ресторан', timezone: 'Asia/Tokyo', expiring_threshold_hours: 72 }
    snapshot.expiring_batches_count = 100; snapshot.attention_total = 102
    server.use(http.get('/api/v1/dashboard', () => Response.json(snapshot)))
    mount(); await ready()
    expect(metric('Истекающие партии')).toBe('100')
    expect(screen.getByRole('status').textContent).toContain('Другой ресторан'); expect(screen.getByRole('status').textContent).toContain('72 ч')
    expect(screen.getByRole('status').textContent).toContain('12:04')
    expect(screen.getAllByRole('time')[0].getAttribute('datetime')).toBe('2001-01-02T03:04:00Z')
  })
  it('separates the full attention total from its 20-row preview', async () => {
    setMockScenario('dashboard-preview'); mount(); await ready()
    expect(screen.getByText('Всего проблемных партий: 25. Показано: 20.')).toBeTruthy()
    expect(screen.getAllByRole('article', { name: /^Партия:/ })).toHaveLength(20)
    expect(Number(metric('Истекающие партии')) + Number(metric('Просроченные партии'))).toBe(25)
  })
  it('preserves server attention and recommendation order, identity and rank', async () => {
    const snapshot = dashboardSnapshot(); snapshot.attention_batches.reverse(); snapshot.hot_offers.reverse(); snapshot.procurement_recommendations.reverse()
    server.use(http.get('/api/v1/dashboard', () => Response.json(snapshot)))
    mount(); await ready()
    expect(screen.getAllByRole('article', { name: /^Партия:/ }).map((node) => node.getAttribute('aria-label'))).toEqual(snapshot.attention_batches.map((batch) => `Партия: ${batch.product_name}, строка ${batch.line_number}`))
    expect(screen.getAllByRole('article', { name: /^Предложение:/ }).map((node) => node.getAttribute('aria-label'))).toEqual(snapshot.hot_offers.map((item) => `Предложение: ${item.dish.name}`))
    expect(screen.getAllByRole('article', { name: /^Закупить:/ }).map((node) => node.getAttribute('aria-label'))).toEqual(snapshot.procurement_recommendations.map((item) => `Закупить: ${item.product_name}`))
    expect(screen.getAllByRole('article', { name: /^Предложение:/ })[0].textContent).toContain('Место: 2')
  })
  it('keeps exact quantities and omits the second countdown', async () => {
    const snapshot = dashboardSnapshot(); snapshot.attention_batches[0].remaining_quantity = '0.001'; snapshot.attention_batches[0].remaining_seconds = 0
    snapshot.procurement_recommendations[0].recommended_quantity = '0.001'
    server.use(http.get('/api/v1/dashboard', () => Response.json(snapshot)))
    mount(); await ready()
    expect(screen.getByRole('article', { name: 'Партия: Молоко, строка 1' }).textContent).toContain('0,001 мл')
    expect(screen.getByRole('article', { name: 'Партия: Молоко, строка 1' }).textContent).not.toContain('0 с до срока')
    expect(screen.getByRole('article', { name: 'Закупить: Молоко' }).textContent).toContain('0,000001 л')
  })
  for (const role of ['admin', 'manager', 'storekeeper', 'order_operator'] as const) {
    it(`${role} hides forbidden blocks and links even in an overprivileged response`, async () => {
      mount(role); await ready()
      expect(!!screen.queryByRole('heading', { name: 'Продвигать блюда' })).toBe(role !== 'storekeeper')
      expect(!!screen.queryByRole('heading', { name: 'Закупить продукты' })).toBe(role !== 'order_operator')
      expect(!!screen.queryByRole('link', { name: 'Открыть: блюда в стоп-листе' })).toBe(role !== 'storekeeper')
      const href = screen.getByRole('link', { name: 'На склад: Молоко, строка 2' }).getAttribute('href')!
      expect(href.includes('supply_id=')).toBe(role !== 'order_operator')
      expect(href).toContain('product_id='); expect(href).toContain('filter=expiring')
      expect(readCount()).toBe(1); expect(requests).toHaveLength(1)
    })
  }
  it('opens the stock filter using the keyboard', async () => {
    const { router, user } = mount(); await ready()
    const link = screen.getByRole('link', { name: 'Открыть: истекающие партии' }); link.focus(); await user.keyboard('{Enter}')
    expect(router.state.location.pathname).toBe('/inventory'); expect(router.state.location.search).toBe('?filter=expiring&sort=nearest_expires_at')
  })
  it('keeps dashboard allowed for every role after login and strips query secrets', () => {
    expect(safeReturnUrl('/dashboard?token=secret&sort=name#secret')).toBe('/dashboard')
    for (const role of ['admin', 'manager', 'storekeeper', 'order_operator'] as const) expect(permittedReturnPath(role, '/dashboard')).toBe('/dashboard')
  })
  it('explains empty attention and both permitted recommendation blocks', async () => {
    setMockScenario('dashboard-empty'); mount(); await screen.findByRole('heading', { name: 'Нет проблемных партий' })
    expect(screen.getByRole('heading', { name: 'Нет горячих предложений' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Закупки не требуются' })).toBeTruthy()
    expect(metric('Истекающие партии')).toBe('0'); expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('shows a skeleton and disables repeated refresh during first load', async () => {
    server.use(http.get('/api/v1/dashboard', async () => { await delay(80); return Response.json(dashboardSnapshot()) }))
    mount(); expect(screen.getByRole('status', { name: 'Загрузка обзора' })).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Обновить обзор' }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryAllByRole('article')).toHaveLength(0); await ready()
  })
  it('recovers a first-load network failure with keyboard retry', async () => {
    setMockScenario('network-error'); const { user } = mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить обзор' })
    expect(screen.queryAllByRole('article')).toHaveLength(0)
    setMockScenario('success'); screen.getByRole('button', { name: 'Попробовать снова' }).focus(); await user.keyboard('{Enter}'); await ready()
  })
  it('labels stale data then suppresses it on 403 without expiring session', async () => {
    const { store, user } = mount(); await ready(); setMockScenario('network-error'); await user.click(screen.getByRole('button', { name: 'Обновить обзор' }))
    await screen.findByText(/Данные могут быть устаревшими/); expect(screen.getAllByRole('article').length).toBeGreaterThan(0)
    setMockScenario('forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить обзор' }))
    await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
    expect(store.getState().session.status).toBe('authenticated')
  })
  it('refreshes on explicit tag invalidation and dish tag changes', async () => {
    const { store } = mount(); await ready(); setMockScenario('dashboard-expired')
    store.dispatch(dashboardApi.util.invalidateTags(['Dashboard'])); await waitFor(() => expect(metric('Истекающие партии')).toBe('0'))
    setMockScenario('success'); store.dispatch(dashboardApi.util.invalidateTags(['Dashboard'])); await waitFor(() => expect(metric('Истекающие партии')).toBe('1'))
    setMockScenario('dashboard-empty'); store.dispatch(dashboardApi.util.invalidateTags(['Dishes'])); await screen.findByRole('heading', { name: 'Нет проблемных партий' })
    expect(readCount()).toBe(4)
  })
  it('refreshes the whole snapshot after published reception and order mutations', async () => {
    const { store } = mount(); await ready(); const missing = Number(metric('Продукты без годного остатка')), stopped = Number(metric('Блюда в стоп-листе'))
    await act(async () => { await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F15-TEST', received_at: new Date().toISOString(), items: [{ product_id: inventoryItems[2].product.id, quantity: '100.000', expires_at: new Date(Date.now() + 3600000).toISOString() }] } })).unwrap() })
    await waitFor(() => expect(Number(metric('Продукты без годного остатка'))).toBe(missing - 1))
    expect(metric('Истекающие партии')).toBe('2')
    await act(async () => { await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body: { items: [{ dish_id: compositionDishes()[0].id, portions: 5, recipe_version: 1 }] } })).unwrap() })
    await waitFor(() => expect(metric('Истекающие партии')).toBe('1'))
    expect(metric('Блюда в стоп-листе')).toBe(String(stopped)); expect(screen.queryByRole('article', { name: 'Партия: Молоко, строка 2' })).toBeNull()
    await screen.findByRole('heading', { name: 'Нет горячих предложений' }); expect(readCount()).toBe(3)
  })
  it('refreshes on return to overview after leaving its subscription', async () => {
    const { router } = mount(); await ready(); await act(() => router.navigate('/inventory'))
    setMockScenario('dashboard-empty'); await act(() => router.navigate('/')); await screen.findByRole('heading', { name: 'Нет проблемных партий' }); expect(readCount()).toBe(2)
  })
  it('refreshes stop-list and procurement previews after reception changes availability', async () => {
    const { store } = mount(); await ready(); const stopped = Number(metric('Блюда в стоп-листе'))
    const milk = screen.getByRole('article', { name: 'Закупить: Молоко' }); expect(milk.textContent).toContain('К закупке: 2,5 л')
    await act(async () => { await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F15-MILK', received_at: new Date().toISOString(), items: [{ product_id: inventoryItems[0].product.id, quantity: '1000.001', expires_at: new Date(Date.now() + 172800000).toISOString() }] } })).unwrap() })
    await waitFor(() => expect(metric('Блюда в стоп-листе')).toBe(String(stopped - 1)))
    expect(milk.textContent).toContain('К закупке: 1,499999 л')
    expect(readCount()).toBe(2)
  })
  it.each(['invalid-schema', 'invalid-json', 'bad-gateway', 'server-error', 'rate-limit'] as const)('rejects %s without rendering partial sections', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить обзор' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('validates preview caps, safe counters, settings, units and full total', () => {
    const snapshot = dashboardSnapshot(); expect(dashboardSchema.safeParse(snapshot).success).toBe(true)
    for (const patch of [{ attention_total: 2 }, { expired_batches_count: -1 }, { expiring_batches_count: Number.MAX_SAFE_INTEGER + 1 }, { settings: { ...snapshot.settings, timezone: 'wrong' } }, { settings: { ...snapshot.settings, expiring_threshold_hours: 0 } }, { meta: { server_time: '2026-10-11' } }, { attention_batches: Array(21).fill(snapshot.attention_batches[0]) }, { hot_offers: Array(4).fill(snapshot.hot_offers[0]) }, { procurement_recommendations: Array(4).fill(snapshot.procurement_recommendations[0]) }]) expect(dashboardSchema.safeParse({ ...snapshot, ...patch }).success).toBe(false)
    expect(dashboardSchema.safeParse({ ...snapshot, attention_batches: [{ ...snapshot.attention_batches[0], remaining_quantity: '0.000' }] }).success).toBe(false)
    expect(dashboardSchema.safeParse({ ...snapshot, attention_batches: [{ ...snapshot.attention_batches[0], unit: 'pcs', remaining_quantity: '1.001' }] }).success).toBe(false)
    for (const key of ['attention_batches', 'hot_offers', 'procurement_recommendations'] as const) expect(dashboardSchema.safeParse({ ...snapshot, [key]: [snapshot[key][0], snapshot[key][0]] }).success).toBe(false)
    expect(dashboardSchema.safeParse({ ...snapshot, extra_counter: 1 }).success).toBe(false)
  })
})
