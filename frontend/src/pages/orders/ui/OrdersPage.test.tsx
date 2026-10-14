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
import { invalidateOrders, orderApi, orderSummarySchema, orderAttemptSchema } from '@/entities/order'
import { orderHandlers } from '@/app/mocks/order-handlers'
import { attemptFixtures, orderFixtures } from '@/app/mocks/order-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { OrdersPage } from './OrdersPage'
import { OrderEntryPage } from './OrderEntryPage'

const server = setupServer(...orderHandlers), NativeRequest = globalThis.Request, requests: URL[] = []
function makeStore(role: Role) {
  return configureStore({
    reducer: { session: sessionReducer, [orderApi.reducerPath]: orderApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: orderFixtures[0].created_by.id, name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(orderApi.middleware),
  })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/orders?orders_search=ТЕСТ', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/orders', element: <OrdersPage /> }, { path: '/orders/new', element: <OrderEntryPage /> }, { path: '/orders/:id', element: <OrderEntryPage /> }], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(orderApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
const table = () => screen.findByRole('table', { name: 'История успешных заказов' })
const attempts = () => screen.findByRole('table', { name: 'История отклонённых попыток' })

describe('published history snapshots', () => {
  it('accepts server historical snapshots and rejects malformed quantities/dates/reasons', () => {
    expect(orderSummarySchema.safeParse(orderFixtures[0]).success).toBe(true)
    expect(orderAttemptSchema.safeParse(attemptFixtures[0]).success).toBe(true)
    expect(orderSummarySchema.safeParse({ ...orderFixtures[0], created_at: '2026-02-30T12:00:00Z' }).success).toBe(false)
    expect(orderAttemptSchema.safeParse({ ...attemptFixtures[0], reason_code: 'NETWORK_ERROR' }).success).toBe(false)
    expect(orderAttemptSchema.safeParse({ ...attemptFixtures[0], shortages: [{ ...attemptFixtures[0].shortages[0], unit: 'pcs', missing_quantity: '0.001' }] }).success).toBe(false)
  })
})
describe('order history in explicit MSW mode', () => {
  it('shows historical names/version, portions, restaurant date, actor and successful number', async () => {
    mount()
    const row = within(await table()).getAllByRole('row')[1]
    for (const text of ['ТЕСТ-101', 'Исторический суп', 'Салат до переименования', 'Рецептура: 3', '2 порц.', '10.10.2026, 12:05', 'Тестовый оператор', 'Выполнен']) expect(row.textContent).toContain(text)
    expect(requests.map((r) => r.pathname)).toEqual(['/api/v1/orders'])
    expect(requests[0].searchParams.get('sort')).toBe('-created_at')
    expect([...requests[0].searchParams.keys()].sort()).toEqual(['page', 'page_size', 'search', 'sort'])
  })
  it.each(['admin', 'manager', 'order_operator'] as const)('shows new-order action for %s', async (role) => { mount('/orders', role); await table(); expect(screen.getByRole('link', { name: 'Новый заказ' })).toBeTruthy() })
  it('hides new-order action from storekeeper even outside the guard', async () => { mount('/orders', 'storekeeper'); await table(); expect(screen.queryByRole('link', { name: 'Новый заказ' })).toBeNull() })
  it('keeps separate pages/filters across tabs and only requests active subscriptions on invalidation', async () => {
    const { store, user, router } = mount('/orders?orders_page_size=20&attempts_page_size=20')
    await table(); await user.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await screen.findByText('Найдено заказов: 26. Страница 2.')
    await user.click(screen.getByRole('tab', { name: 'Отклонённые попытки' }))
    await attempts(); await user.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await screen.findByText('Найдено отказов: 25. Страница 2.')
    const count = requests.length
    store.dispatch(invalidateOrders())
    await waitFor(() => expect(requests.length).toBeGreaterThan(count))
    expect(requests.slice(count).map((r) => r.pathname)).toEqual(['/api/v1/order-attempts'])
    await user.selectOptions(screen.getByLabelText('Причина отказа'), 'RECIPE_CHANGED')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Найдено отказов: 1. Страница 1.')
    expect(router.state.location.search).toContain('orders_page=2')
    expect(requests.at(-1)?.searchParams.get('reason_code')).toBe('RECIPE_CHANGED')
    await user.click(screen.getByRole('tab', { name: 'Успешные заказы' }))
    await screen.findByText('Найдено заказов: 26. Страница 2.')
    expect(requests.at(-1)?.searchParams.has('reason_code')).toBe(false)
  })
  it('expands shortage snapshots with exact quantities and attempt identity without order number', async () => {
    const { user } = mount('/orders?tab=attempts&attempts_reason_code=INSUFFICIENT_STOCK&attempts_search=000000000200')
    const list = await attempts(), row = within(list).getAllByRole('row')[1]
    expect(row.textContent).toContain(attemptFixtures[0].id)
    expect(within(list).queryByRole('link')).toBeNull()
    await user.click(within(row).getByText('Причина и дефицит'))
    expect(row.querySelector('details')?.open).toBe(true)
    expect(row.textContent).toContain('1200,001 г')
    expect(row.textContent).toContain('Весь физический остаток просрочен')
    expect(row.textContent).not.toContain('ТЕСТ-101')
  })
  it.each(['RECIPE_CHANGED', 'DISH_UNAVAILABLE'])('renders saved issue details for %s', async (reason) => {
    mount(`/orders?tab=attempts&attempts_reason_code=${reason}`)
    const list = await attempts()
    expect(list.textContent).toContain(reason === 'RECIPE_CHANGED' ? 'Рецептура изменилась после проверки' : 'Блюдо отключено')
  })
  it('uses literal historical dish/number search and inclusive/exclusive bounds', async () => {
    mount('/orders?orders_search=ГРАНИЦА&orders_from=2026-10-09T21:00:00Z&orders_to=2026-10-10T21:00:00Z')
    const list = await table()
    expect(within(list).getByRole('link', { name: 'Заказ ГРАНИЦА-С' })).toBeTruthy()
    expect(within(list).queryByRole('link', { name: 'Заказ ГРАНИЦА-ДО' })).toBeNull()
  })
  it('validates period fields and makes no extra request', async () => {
    const { user } = mount(); await table()
    await user.type(screen.getByLabelText('Создана с (включительно)'), '2026-10-11T00:00')
    await user.type(screen.getByLabelText('Создана до (не включая)'), '2026-10-10T00:00')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Начало периода должно быть не позже конца.')
    expect(screen.getByLabelText('Создана до (не включая)').getAttribute('aria-invalid')).toBe('true')
    expect(requests).toHaveLength(1)
  })
  it.each(['orders_from=bad', 'tab=attempts&attempts_reason_code=NETWORK_ERROR'])('does not request invalid filters and resets: %s', async (query) => {
    const { user } = mount(`/orders?${query}`)
    expect(screen.getByRole('heading', { name: 'Некорректные фильтры' })).toBeTruthy(); expect(requests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
    await screen.findByRole('table')
  })
  it('returns from order and new-order destinations with both tabs intact and refreshes', async () => {
    const { user, router } = mount('/orders?orders_search=ТЕСТ&attempts_page=2')
    await user.click(within(await table()).getByRole('link', { name: 'Заказ ТЕСТ-101' }))
    await screen.findByRole('heading', { name: 'Детали заказа' }); const count = requests.length
    await user.click(screen.getByRole('link', { name: 'К истории заказов' })); await table()
    await waitFor(() => expect(requests.length).toBeGreaterThan(count))
    expect(router.state.location.search).toContain('attempts_page=2')
    await user.click(screen.getByRole('link', { name: 'Новый заказ' })); await screen.findByRole('heading', { name: 'Новый заказ' })
    await user.click(screen.getByRole('link', { name: 'К истории заказов' })); await table()
  })
  it.each(['orders', 'attempts'] as const)('distinguishes empty, no results and out-of-range %s page', async (tab) => {
    setMockScenario('orders-empty'); const { store, user } = mount(`/orders?tab=${tab}`)
    await screen.findByRole('heading', { name: tab === 'orders' ? 'Заказов пока нет' : 'Отклонённых попыток пока нет' })
    setMockScenario('success'); store.dispatch(invalidateOrders()); await screen.findByRole('table')
    await user.type(screen.getByLabelText(tab === 'orders' ? 'Номер заказа или название блюда' : 'ID попытки или название блюда'), 'нет такого{Enter}')
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
  })
  it('recovers an out-of-range page preserving server total', async () => {
    const { user } = mount('/orders?tab=attempts&attempts_page=3&attempts_page_size=20')
    await screen.findByRole('heading', { name: 'На этой странице нет записей' })
    expect(screen.getByText('Найдено отказов: 25. Страница 3.')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'К первой странице' })); await attempts()
  })
  it.each(['orders', 'attempts'] as const)('retains current %s on network failure, hides changed-filter data and retries without invented refusals', async (tab) => {
    const { store, user } = mount(`/orders?tab=${tab}`); await screen.findByRole('table')
    setMockScenario('network-error'); store.dispatch(invalidateOrders())
    await screen.findByRole('heading', { name: `Не удалось загрузить историю ${tab === 'orders' ? 'заказов' : 'отказов'}` })
    expect(screen.getByRole('table')).toBeTruthy()
    await user.type(screen.getByLabelText(tab === 'orders' ? 'Номер заказа или название блюда' : 'ID попытки или название блюда'), 'нет такого{Enter}')
    await waitFor(() => expect(screen.queryByRole('table')).toBeNull())
    await screen.findByText('Попробуйте ещё раз. Ошибка загрузки не является сохранённым отказом заказа.')
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByRole('heading', { name: 'Ничего не найдено' })
  })
  it.each(['orders', 'attempts'] as const)('first-load error is not empty %s history; 403 hides cache without logging out', async (tab) => {
    setMockScenario('server-error'); const { store, user } = mount(`/orders?tab=${tab}`)
    await screen.findByRole('button', { name: 'Попробовать снова' }); expect(screen.queryByRole('table')).toBeNull()
    expect(screen.queryByRole('heading', { name: 'Отклонённых попыток пока нет' })).toBeNull()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByRole('table')
    setMockScenario('forbidden'); store.dispatch(invalidateOrders()); await screen.findByRole('heading', { name: 'Недостаточно прав' })
    expect(screen.queryByRole('table')).toBeNull(); expect(store.getState().session.status).toBe('authenticated')
  })
  it.each([{ page: 2 }, { page_size: 20 }, { total: 0 }, { items: [orderFixtures[0], orderFixtures[0]] }, { items: [{ ...orderFixtures[0], status: 'rejected' }] }])('rejects invalid envelopes: %j', async (patch) => {
    server.use(http.get('/api/v1/orders', () => Response.json({ items: [orderFixtures[0]], page: 1, page_size: 50, total: 1, meta: {}, ...patch })))
    mount('/orders'); await screen.findByRole('heading', { name: 'Не удалось загрузить историю заказов' }); expect(screen.queryByRole('table')).toBeNull()
  })
  it('a preliminary check refusal is never added to the saved attempt history', async () => {
    server.use(http.post('/api/v1/orders/check', () => Response.json({ can_create: false, items: attemptFixtures[0].items, shortages: attemptFixtures[0].shortages, requirements: [], issues: [], meta: {} })))
    const { store } = mount('/orders?tab=attempts'); await attempts()
    const response = await fetch(new Request('/api/v1/orders/check', { method: 'POST', body: JSON.stringify({ items: [{ dish_id: attemptFixtures[0].items[0].dish_id, portions: 2 }] }), headers: { 'Content-Type': 'application/json' } }))
    expect((await response.json()).can_create).toBe(false)
    store.dispatch(invalidateOrders()); await waitFor(() => expect(requests.filter((r) => r.pathname === '/api/v1/order-attempts')).toHaveLength(2))
    await screen.findByText('Найдено отказов: 25. Страница 1.')
  })
})
