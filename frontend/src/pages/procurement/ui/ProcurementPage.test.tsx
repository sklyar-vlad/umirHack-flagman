// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { procurementApi, procurementListSchema, readProcurementSearch, procurementSearchParams, changeProcurementSearch } from '@/entities/procurement'
import { inventoryHandlers } from '@/app/mocks/inventory-handlers'
import { RoleGate } from '@/features/access'
import { sections } from '@/entities/session'
import { RecommendationsLanding } from './RecommendationsLanding'
import { initialSessionState, sessionReducer, type Role } from '@/entities/session'
import { orderApi } from '@/entities/order'
import { supplyApi } from '@/entities/supply'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { procurementHandlers } from '@/app/mocks/procurement-handlers'
import { procurementItems } from '@/app/mocks/procurement-fixtures'
import { compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { receptionHandlers } from '@/app/mocks/reception-handlers'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { inventoryItems } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { ProcurementPage } from './ProcurementPage'

const server = setupServer(...procurementHandlers, ...inventoryHandlers, ...receptionHandlers, ...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: URL[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [procurementApi.reducerPath]: procurementApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(procurementApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/recommendations/procurement', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/recommendations/procurement', element: <RoleGate section={sections.find((section) => section.id === 'procurement')!}><ProcurementPage /></RoleGate> }, { path: '/recommendations', element: <RecommendationsLanding /> }], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const card = (name = 'Молоко') => screen.findByRole('article', { name: `Закупить: ${name}` })
const readCount = () => requests.filter((url) => url.pathname === '/api/v1/procurement-recommendations').length
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); resetReceptionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(procurementApi.util.resetApiState())); server.resetHandlers(); vi.useRealTimers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('published procurement UI in explicit MSW', () => {
  it('allowlists contract parameters and safe login returns', () => {
    const state = readProcurementSearch(new URLSearchParams(`search=%25_%5C&page=2&page_size=1&category_id=${inventoryItems[0].product.category.id}&sort=name&password=secret`))
    expect(state.page).toBe(2); expect(state.category_id).toBe(inventoryItems[0].product.category.id)
    expect(changeProcurementSearch(state, { category_id: '' }).page).toBe(1)
    expect(readProcurementSearch(new URLSearchParams('page=0&page_size=101&category_id=invalid'))).toEqual({ search: '', category_id: '', page: 1, page_size: 20 })
    expect(safeReturnUrl(`/recommendations/procurement?${procurementSearchParams(state)}&password=secret`)).toBe(`/recommendations/procurement?${procurementSearchParams(state)}`)
    expect(safeReturnUrl(`/recommendations?tab=procurement&search=milk&token=secret`)).toBe('/recommendations?search=milk&tab=procurement')
    expect(safeReturnUrl(`/supplies/new?product_id=${inventoryItems[0].product.id}&quantity=999&token=secret`)).toBe(`/supplies/new?product_id=${inventoryItems[0].product.id}`)
  })
  it('keeps server priority and displays exact units, reason and affected dishes', async () => {
    mount(); const milk = await card()
    expect(screen.getAllByRole('article').map((node) => node.getAttribute('aria-label'))).toEqual(procurementItems().slice(0, 20).map((item) => `Закупить: ${item.product_name}`))
    for (const text of ['1,5 л', '4 л', '2,5 л', '0,2 л', 'Ниже целевого запаса', 'Молочный суп']) expect(milk.textContent).toContain(text)
    expect(within(milk).getByRole('link', { name: 'Молочный суп' }).getAttribute('href')).toContain('search=')
    expect(requests.filter((url) => url.pathname.includes('procurement')).every((url) => !url.searchParams.has('sort'))).toBe(true)
    expect(requests.some((url) => /orders|forecast/.test(url.pathname))).toBe(false)
  })
  it('does not reorder the response on the client', async () => {
    const items = procurementItems().slice(0, 3).reverse()
    server.use(http.get('/api/v1/procurement-recommendations', () => Response.json({ items, total: 3, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); await card()
    expect(screen.getAllByRole('article').map((node) => node.getAttribute('aria-label'))).toEqual(items.map((item) => `Закупить: ${item.product_name}`))
  })
  it('preserves thousandths, aggregate stock and integral pieces', async () => {
    const items = procurementItems(), egg = items.find((item) => item.unit === 'pcs')!
    const snapshot = { ...items[0], available_quantity: '199999999999.998', target_stock: '4000.001', recommended_quantity: '0.001' }
    server.use(http.get('/api/v1/procurement-recommendations', () => Response.json({ items: [snapshot, egg], total: 2, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); expect((await card()).textContent).toContain('0,000001 л'); expect((await card()).textContent).toContain('199999999,999998 л')
    expect((await card('Яйцо')).textContent).toContain('10 шт.')
  })
  it('distinguishes expired-only stock and an unset target', async () => {
    mount('/recommendations/procurement?search=Творог'); const expired = await card('Творог — только просрочка')
    expect(expired.textContent).toContain('Остались только просроченные партии'); expect(expired.textContent).toContain('3 кг')
    expect(screen.queryByRole('article', { name: 'Закупить: Сахар' })).toBeNull()
    expect(procurementItems().some((item) => item.product_name === 'Мука')).toBe(false)
  })
  it.each(['admin', 'manager', 'storekeeper'] as const)('filters related links for %s', async (role) => {
    mount(undefined, role); const milk = await card()
    expect(within(milk).getByRole('link', { name: 'На склад: Молоко' }).getAttribute('href')).toContain(inventoryItems[0].product.id)
    expect(within(milk).getByRole('link', { name: 'Принять поставку: Молоко' }).getAttribute('href')).toBe(`/supplies/new?product_id=${inventoryItems[0].product.id}`)
    expect(within(milk).queryByRole('link', { name: 'Молочный суп' }) !== null).toBe(role !== 'storekeeper')
  })
  it('denies operator before mounting queries, including the tab URL', async () => {
    mount('/recommendations?tab=procurement', 'order_operator')
    await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it('redirects the tab URL preserving search and category', async () => {
    const { router } = mount('/recommendations?tab=procurement&search=Молоко&token=secret')
    await card(); expect(router.state.location.pathname).toBe('/recommendations/procurement'); expect(router.state.location.search).not.toContain('token'); expect(screen.getAllByRole('article')).toHaveLength(1)
  })
  it('supports pagination, keyboard search, category changes and history', async () => {
    const { user, router } = mount('/recommendations/procurement?page=2'); await screen.findByText(/Страница 2. Снимок/)
    await user.type(screen.getByLabelText('Поиск продукта'), 'МОЛОКО{Enter}'); await card()
    expect(router.state.location.search).not.toContain('page='); expect(screen.getAllByRole('article')).toHaveLength(1)
    await user.selectOptions(screen.getByLabelText('Категория'), inventoryItems[4].product.category.id); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await act(() => router.navigate(-1)); await card()
    await user.click(screen.getByRole('button', { name: 'Сбросить фильтры' })); await screen.findByText(/Продуктов к закупке: 23/)
  })
  it('isolates delayed responses from the current filters', async () => {
    let release!: () => void
    const waiting = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('/api/v1/procurement-recommendations', async ({ request }) => {
      const search = new URL(request.url).searchParams.get('search') ?? '', items = procurementItems().filter((item) => item.product_name.includes(search))
      if (search === 'Молоко') await waiting
      return Response.json({ items, total: items.length, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })
    }))
    const { router } = mount('/recommendations/procurement?search=Молоко'); await waitFor(() => expect(readCount()).toBe(1))
    await act(() => router.navigate('/recommendations/procurement?search=Яйцо')); await card('Яйцо')
    await act(async () => { release(); await waiting }); expect(screen.queryByRole('article', { name: 'Закупить: Молоко' })).toBeNull()
  })
  it('offers first-load retry, loading and stale data on network errors', async () => {
    setMockScenario('network-error'); const { store, user } = mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить закупки' }); expect(screen.queryByRole('article')).toBeNull()
    setMockScenario('success'); within(screen.getAllByRole('alert').find((alert) => alert.textContent?.includes('Не удалось загрузить закупки'))!).getByRole('button', { name: 'Попробовать снова' }).focus(); await user.keyboard('{Enter}'); await card()
    setMockScenario('slow'); store.dispatch(procurementApi.util.invalidateTags(['Procurement']))
    await screen.findByRole('status', { name: 'Загрузка закупок' }); expect((screen.getByRole('button', { name: 'Обновить закупки' }) as HTMLButtonElement).disabled).toBe(true)
    await waitFor(() => expect(screen.queryByRole('status', { name: 'Загрузка закупок' })).toBeNull(), { timeout: 3000 })
    setMockScenario('network-error'); store.dispatch(procurementApi.util.invalidateTags(['Procurement'])); await screen.findByText(/Данные могут быть устаревшими/); expect(await card()).toBeTruthy()
  })
  it('recovers category failures without hiding procurement data', async () => {
    setMockScenario('categories-error'); const { user } = mount(); await card()
    await screen.findByRole('heading', { name: 'Не удалось загрузить категории' }); expect((screen.getByLabelText('Категория') as HTMLSelectElement).disabled).toBe(true)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByRole('option', { name: 'Молочные продукты' })
    expect((screen.getByLabelText('Категория') as HTMLSelectElement).disabled).toBe(false)
  })
  it('hides cached records on 403 without expiring the session', async () => {
    const { store } = mount(); await card(); setMockScenario('forbidden'); store.dispatch(procurementApi.util.invalidateTags(['Procurement']))
    await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByRole('article')).toBeNull(); expect(store.getState().session.status).toBe('authenticated')
  })
  it.each(['invalid-schema', 'invalid-json'] as const)('rejects %s', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить закупки' }); expect(screen.queryByRole('heading', { name: 'Закупки не требуются' })).toBeNull()
  })
  it('explains empty, no-match and out-of-range pages', async () => {
    setMockScenario('procurement-empty'); const { router, user } = mount(); await screen.findByRole('heading', { name: 'Закупки не требуются' }); expect(screen.getByText(/Продукты без цели/)).toBeTruthy()
    setMockScenario('success'); await act(() => router.navigate('/recommendations/procurement?search=%25_%5C')); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await act(() => router.navigate('/recommendations/procurement?page=9')); await screen.findByRole('heading', { name: 'На этой странице нет закупок' }); await user.click(screen.getByRole('button', { name: 'К первой странице' })); await card()
  })
  it('refreshes after typed reception and order mutations', async () => {
    const { store } = mount('/recommendations/procurement?search=Молоко'); expect((await card()).textContent).toContain('2,5 л')
    const now = new Date().toISOString(), expires = new Date(Date.now() + 86400000).toISOString()
    await act(async () => { await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F13-TEST', received_at: now, items: [{ product_id: inventoryItems[0].product.id, quantity: '1000.001', expires_at: expires }] } })).unwrap() })
    await waitFor(() => expect(screen.getByRole('article').textContent).toContain('1,499999 л'))
    await act(async () => { await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body: { items: [{ dish_id: compositionDishes()[0].id, portions: 1, recipe_version: 1 }] } })).unwrap() })
    await waitFor(() => expect(screen.getByRole('article').textContent).toContain('1,599999 л')); expect(readCount()).toBe(3)
  })
  it('refreshes recipes, focus and reconnect', async () => {
    const { store } = mount('/recommendations/procurement?search=Молоко'); await card()
    setMockScenario('stop-list-recipe'); store.dispatch(procurementApi.util.invalidateTags(['Dishes'])); await waitFor(() => expect(screen.queryByRole('link', { name: 'Молочный суп' })).toBeNull())
    setMockScenario('success'); store.dispatch(procurementApi.internalActions.onFocus()); await screen.findByRole('link', { name: 'Молочный суп' })
    setMockScenario('procurement-expired'); store.dispatch(procurementApi.internalActions.onOnline()); await waitFor(() => expect(screen.getByRole('article').textContent).toContain('К закупке: 4 л'))
  })
  it('rejects null/zero targets, incompatible units, fractional pieces and duplicate rows', () => {
    const items = procurementItems().slice(0, 2), data = { items, total: 2, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } }
    expect(procurementListSchema.safeParse(data).success).toBe(true)
    for (const patch of [{ target_stock: null }, { target_stock: '0.000' }, { recommended_quantity: '0.000' }, { display_unit: 'kg' }, { reason_codes: [] }]) expect(procurementListSchema.safeParse({ ...data, items: [{ ...items[0], ...patch }] }).success).toBe(false)
    expect(procurementListSchema.safeParse({ ...data, items: [items[0], items[0]] }).success).toBe(false)
    const egg = procurementItems().find((item) => item.unit === 'pcs')!
    expect(procurementListSchema.safeParse({ ...data, items: [{ ...egg, available_quantity: '10.001' }] }).success).toBe(false)
  })
  it('rejects mismatched pagination', async () => {
    server.use(http.get('/api/v1/procurement-recommendations', () => Response.json({ items: [], total: 0, page: 2, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить закупки' })
  })
})
