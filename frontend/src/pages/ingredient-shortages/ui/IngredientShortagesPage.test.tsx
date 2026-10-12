// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { shortageApi, shortageRowSchema, shortageSearchError, shortageSearchParams, changeShortageSearch, defaultShortageSearch, readShortageSearch } from '@/entities/ingredient-shortage'
import { orderApi } from '@/entities/order/api/order-api'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { AuthContext, type SessionController } from '@/features/auth'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { shortageHandlers } from '@/app/mocks/shortage-handlers'
import { mockId } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { IngredientShortagesPage } from './IngredientShortagesPage'

const server = setupServer(...shortageHandlers, ...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [shortageApi.reducerPath]: shortageApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(800), name: 'Сотрудник', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Кладовая', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } }, middleware: (get) => get().concat(shortageApi.middleware) })
}
function mount(path = '/analytics/ingredient-shortages', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/analytics/ingredient-shortages', element: <RoleGate section={sections.find((s) => s.id === 'analytics')!}><IngredientShortagesPage /></RoleGate> }], { initialEntries: [path] })
  const auth: SessionController = { restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), hasAccess: () => true, getVersion: () => 0, registerSubscription: () => () => false }
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { router, user: userEvent.setup(), store }
}
const ready = () => screen.findByText('Групп продуктов и единиц: 1. Страница 1.')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(shortageApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

const row = { product_id: mockId(10), product_name: 'Историческое молоко', unit: 'ml' as const, rejected_attempts_count: 2, total_missing_quantity: '199999999999.998' }
const endpoint = '/api/v1/analytics/ingredient-shortages'
describe('F28 ingredient shortage report', () => {
  it('validates exact unbounded aggregates, safe counts, positive demand and integral pieces', () => {
    expect(shortageRowSchema.safeParse(row).success).toBe(true)
    for (const patch of [{ unit: 'pcs' }, { total_missing_quantity: '0.000' }, { total_missing_quantity: '-1.000' }, { total_missing_quantity: '1.0001' }, { total_missing_quantity: '1e3' }, { rejected_attempts_count: 0 }, { rejected_attempts_count: 1.5 }, { rejected_attempts_count: Number.MAX_SAFE_INTEGER + 1 }, { token: 'secret' }]) expect(shortageRowSchema.safeParse({ ...row, ...patch }).success).toBe(false)
    expect(shortageRowSchema.safeParse({ ...row, unit: 'pcs', total_missing_quantity: '999999999999.000' }).success).toBe(true)
  })
  it('allowlists URL fields, preserves exact periods and resets page', () => {
    const values = readShortageSearch(new URLSearchParams('page=3&from=2026-10-10T08%3A00%3A00.000000001Z&token=secret&sort=product_name&product_id=bad'))
    expect(shortageSearchError(values)).toBeNull()
    expect([...shortageSearchParams(values).keys()]).toEqual(['from', 'page'])
    expect(changeShortageSearch(values, { to: '2026-10-11T00:00:00Z' }).page).toBe(1)
    expect(safeReturnUrl('/analytics/ingredient-shortages?page=3&product_id=bad&token=secret')).toBe('/analytics/ingredient-shortages?page=3')
    expect(shortageSearchError({ ...values, to: '2026-10-09T00:00:00Z' })).toBeTruthy()
  })
  it('shows historical unit groups, exact large totals and server priority without write requests', async () => {
    setMockScenario('shortages-demo'); mount(); await screen.findByText('Групп продуктов и единиц: 27. Страница 1.')
    expect(screen.getAllByText('200000027600,021 г').length).toBeGreaterThan(0)
    expect(screen.getAllByText('3200,001 мл').length).toBeGreaterThan(0)
    expect(screen.getAllByText('2 шт.').length).toBeGreaterThan(0)
    expect(requests.every((r) => r.method === 'GET' && new URL(r.url).pathname === endpoint && [...new URL(r.url).searchParams.keys()].every((key) => ['page', 'page_size', 'from', 'to'].includes(key)))).toBe(true)
  })
  it('composes supported history filters with the same exact period and admin product links', async () => {
    mount('/analytics/ingredient-shortages?from=2026-10-10T09%3A00%3A00.000000001Z&to=2026-10-10T10%3A00%3A00Z'); await ready()
    const link = new URL(screen.getAllByRole('link', { name: 'История отказов за период' })[0].getAttribute('href')!, 'https://internal.test')
    expect(link.searchParams.get('tab')).toBe('attempts'); expect(link.searchParams.get('attempts_reason_code')).toBe('INSUFFICIENT_STOCK')
    expect(link.searchParams.get('attempts_from')).toBe('2026-10-10T09:00:00.000000001Z'); expect(link.searchParams.get('attempts_to')).toBe('2026-10-10T10:00:00Z')
    expect(link.searchParams.has('product_id')).toBe(false); expect(link.searchParams.has('attempts_search')).toBe(false)
    expect(screen.getAllByRole('link', { name: 'Карточка продукта' })[0].getAttribute('href')).toBe(`/admin/products/${mockId(700)}/edit`)
    expect(screen.getAllByRole('link', { name: 'Продукт на складе' })[0].getAttribute('href')).toContain(`product_id=${mockId(700)}`)
  })
  it('allows managers and hides admin product links', async () => { mount('/analytics/ingredient-shortages', 'manager'); await ready(); expect(screen.queryAllByRole('link', { name: 'Карточка продукта' })).toHaveLength(0); expect(screen.getAllByRole('link', { name: 'Продукт на складе' }).length).toBeGreaterThan(0) })
  it.each(['storekeeper', 'order_operator'] as const)('blocks %s before the query', async (role) => { mount('/analytics/ingredient-shortages', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0) })
  it.each(['from=yesterday', 'from=2026-10-11T00:00:00Z&to=2026-10-10T00:00:00Z'])('blocks invalid period %s', async (search) => { mount(`/analytics/ingredient-shortages?${search}`); await screen.findByRole('heading', { name: 'Некорректные фильтры' }); expect(requests).toHaveLength(0) })
  it('pages, resets the page for a new filter and restores the URL on back', async () => {
    setMockScenario('shortages-demo'); const { user, router } = mount('/analytics/ingredient-shortages?page_size=20'); await screen.findByText('Групп продуктов и единиц: 27. Страница 1.')
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await screen.findByText('Групп продуктов и единиц: 27. Страница 2.')
    await user.type(screen.getByLabelText('Отказы с (включительно)'), '2026-10-11T00:00'); await user.click(screen.getByRole('button', { name: 'Найти' })); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    expect(new URLSearchParams(router.state.location.search).has('page')).toBe(false)
    await router.navigate(-1); await screen.findByText('Групп продуктов и единиц: 27. Страница 2.')
  })
  it('uses inclusive from/exclusive to and retains unchanged nanoseconds in the form', async () => {
    const { user, router } = mount('/analytics/ingredient-shortages?from=2026-10-10T09%3A10%3A00Z&to=2026-10-10T09%3A10%3A00.000000001Z'); await ready()
    expect(screen.getAllByText('1200,001 г').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: 'Найти' })); expect(new URLSearchParams(router.state.location.search).get('to')).toBe('2026-10-10T09:10:00.000000001Z')
    await router.navigate('/analytics/ingredient-shortages?to=2026-10-10T09%3A10%3A00Z'); await ready()
    expect((await screen.findAllByText('26400,022 г')).length).toBeGreaterThan(0)
  })
  it('keeps same-period stale data after a network/server failure and suppresses it on 403', async () => {
    const { user } = mount(); await ready(); setMockScenario('shortages-error'); await user.click(screen.getByRole('button', { name: 'Обновить отчёт' })); await screen.findByText(/Данные могут быть устаревшими/)
    expect(screen.getByText('Групп продуктов и единиц: 1. Страница 1.')).toBeTruthy()
    setMockScenario('shortages-forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить отчёт' })); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByText('Групп продуктов и единиц: 1. Страница 1.')).toBeNull()
  })
  it('never displays previous parameter data while a new period loads', async () => {
    const { router } = mount(); await ready(); setMockScenario('shortages-slow'); await router.navigate('/analytics/ingredient-shortages?from=2026-10-11T00:00:00Z'); await screen.findByRole('status', { name: 'Загрузка недостачи' })
    expect(screen.queryByText('Групп продуктов и единиц: 1. Страница 1.')).toBeNull()
  })
  it.each(['shortages-empty', 'shortages-invalid'] as const)('renders %s', async (scenario) => { setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: scenario === 'shortages-empty' ? 'Отказов из-за недостачи пока нет' : 'Не удалось загрузить недостачу' }) })
  it.each(['duplicates', 'wrong-page', 'wrong-size', 'bad-count'] as const)('rejects %s before caching', async (kind) => {
    server.use(http.get(endpoint, () => Response.json({ items: kind === 'duplicates' ? [row, row] : [{ ...row, ...(kind === 'bad-count' ? { rejected_attempts_count: 0 } : {}) }], total: 2, page: kind === 'wrong-page' ? 2 : 1, page_size: kind === 'wrong-size' ? 20 : 50, meta: {} })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить недостачу' })
  })
  it('offers first page when the requested page is empty', async () => { mount('/analytics/ingredient-shortages?page=99'); await screen.findByRole('heading', { name: 'На этой странице нет недостачи' }); expect(screen.getByRole('button', { name: 'К первой странице' })).toBeTruthy() })
  it('counts one saved POST refusal, replay once, and no checks/success/other reasons; refreshes subscribed analytics', async () => {
    const { store } = mount(); await ready()
    const body = { items: [{ dish_id: mockId(800), portions: 1, recipe_version: 1 }] }, key = mockId(5000)
    const report = store.dispatch(shortageApi.endpoints.ingredientShortages.initiate(defaultShortageSearch))
    const before = await report.unwrap(); expect(before.items).toHaveLength(1)
    for (let i = 0; i < 3; i++) await store.dispatch(orderApi.endpoints.checkOrder.initiate(body)).unwrap()
    expect((await report.refetch().unwrap()).items).toEqual(before.items)
    setMockScenario('order-shortage'); const first = await store.dispatch(orderApi.endpoints.createOrder.initiate({ key, body }))
    expect(first.error).toMatchObject({ status: 409, code: 'INSUFFICIENT_STOCK' })
    await waitFor(() => expect(shortageApi.endpoints.ingredientShortages.select(defaultShortageSearch)(store.getState()).data?.total).toBe(2))
    const after = await report.refetch().unwrap(); expect(after.items.find((r) => r.product_id === mockId(10))?.rejected_attempts_count).toBe(1)
    const replay = await store.dispatch(orderApi.endpoints.createOrder.initiate({ key, body })); expect(replay.error).toMatchObject({ status: 409, code: 'INSUFFICIENT_STOCK', details: 'error' in first && first.error && 'details' in first.error ? first.error.details : {} })
    expect((await report.refetch().unwrap()).items).toEqual(after.items)
    setMockScenario('success'); await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: mockId(5001), body })).unwrap()
    expect((await report.refetch().unwrap()).items).toEqual(after.items)
    setMockScenario('order-final-inactive'); await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: mockId(5002), body }))
    expect((await report.refetch().unwrap()).items).toEqual(after.items)
    report.unsubscribe()
  })
})
