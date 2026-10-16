// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { dishApi, stopListSchema, readStopListSearch, stopListSearchParams, changeStopListSearch } from '@/entities/dish'
import { initialSessionState, sessionReducer, type Role } from '@/entities/session'
import { orderApi } from '@/entities/order'
import { supplyApi } from '@/entities/supply'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { stopListHandlers } from '@/app/mocks/stop-list-handlers'
import { stopListDishes } from '@/app/mocks/stop-list-fixtures'
import { compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { receptionHandlers } from '@/app/mocks/reception-handlers'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { inventoryItems } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { createLiveEvents } from '@/shared/api/live-events'
import { StopListPage } from './StopListPage'

const server = setupServer(...stopListHandlers, ...receptionHandlers, ...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: URL[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [dishApi.reducerPath]: dishApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(dishApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/stop-list', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/stop-list', element: <StopListPage /> }], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const card = (name = 'Молочный суп') => screen.findByRole('article', { name: `Блюдо: ${name}` })
const readCount = () => requests.filter((url) => url.pathname === '/api/v1/stop-list').length
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); resetReceptionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dishApi.util.resetApiState())); server.resetHandlers(); vi.useRealTimers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('published per-portion stop-list in explicit MSW mode', () => {
  it('allowlists filters and login return, resetting pagination on edits', () => {
    const state = readStopListSearch(new URLSearchParams('search=%25_%5C&page=2&page_size=1&availability=all&password=secret'))
    expect(state).toEqual({ search: '%_\\', page: 2, page_size: 1 })
    expect(changeStopListSearch(state, { search: 'суп' }).page).toBe(1)
    expect(readStopListSearch(new URLSearchParams('page=0&page_size=101'))).toEqual({ search: '', page: 1, page_size: 20 })
    expect(safeReturnUrl(`/stop-list?${stopListSearchParams(state)}&password=secret`)).toBe(`/stop-list?${stopListSearchParams(state)}`)
  })
  it('shows exact per-portion quantities without basket requests or disabled dishes', async () => {
    mount(); const result = await card()
    const shortage = within(result).getByRole('region', { name: 'Дефицит: Молоко' })
    for (const value of ['1500,001 мл', '1500 мл', '0,001 мл', '200 мл']) expect(shortage.textContent).toContain(value)
    expect(requests.every((url) => url.pathname === '/api/v1/stop-list')).toBe(true)
    expect(Object.fromEntries(requests[0].searchParams)).toEqual({ search: '', page: '1', page_size: '20', sort: 'name' })
    expect(screen.queryByRole('heading', { name: 'Отключённое блюдо' })).toBeNull()
  })
  it('distinguishes zero stock, expired-only stock and missing recipe', async () => {
    mount(); const expired = await card('Крем — только просрочка'), empty = await card('Десерт без остатка'), noRecipe = await card('Без рецептуры')
    expect(expired.textContent).toContain('Остались только просроченные партии'); expect(expired.textContent).toContain('300 мл')
    expect(empty.textContent).toContain('Недостаточно годного остатка'); expect(empty.textContent).toContain('20,001 г')
    expect(noRecipe.textContent).toContain('Рецептура отсутствует'); expect(noRecipe.textContent).toContain('Обратитесь к администратору')
    expect(within(noRecipe).queryByRole('heading', { name: 'Дефицит на одну порцию' })).toBeNull()
  })
  it('keeps every recipe ingredient and exact server shortage, including aggregate stock and pieces', async () => {
    const dish = stopListDishes().find((item) => item.name === 'Молочный суп')!
    const sugar = inventoryItems[2].product, eggs = inventoryItems[4].product
    const ingredient = (product: typeof sugar, quantity: string) => ({ product_id: product.id, product_name: product.name, unit: product.unit, quantity_per_portion: quantity })
    const snapshot = { ...dish, recipe: [...dish.recipe, ingredient(sugar, '20.001'), ingredient(eggs, '12.000')],
      shortages: [...dish.shortages, { product_id: eggs.id, product_name: eggs.name, unit: eggs.unit, required_quantity: '12.000', available_quantity: '10.000', missing_quantity: '2.000', expired_quantity: '0.000', reason_code: 'ingredient_shortage' as const }] }
    server.use(http.get('/api/v1/stop-list', () => Response.json({ items: [snapshot], total: 1, page: 1, page_size: 20, meta: dish.meta })))
    mount(); const result = await card()
    expect(result.textContent).toContain('Сахар — 20,001 г'); expect(result.textContent).toContain('Яйцо — 12 шт.')
    const deficit = within(result).getByRole('region', { name: 'Дефицит: Яйцо' })
    expect(deficit.textContent).toContain('10 шт.'); expect(deficit.textContent).toContain('2 шт.')
    expect(within(result).getAllByRole('region')).toHaveLength(2)
    expect(stopListSchema.safeParse({ items: [{ ...snapshot, shortages: [{ ...dish.shortages[0], available_quantity: '199999999999.998' }] }], total: 1, page: 1, page_size: 20, meta: dish.meta }).success).toBe(true)
  })
  it.each(['admin', 'manager', 'order_operator'] as const)('filters related links for %s', async (role) => {
    mount(undefined, role); const soup = await card(), noRecipe = await card('Без рецептуры')
    const link = within(soup).getByRole('link', { name: 'На склад: Молоко' })
    expect(new URL(link.getAttribute('href')!, 'https://internal.test').searchParams.get('product_id')).toBe(inventoryItems[0].product.id)
    expect(within(soup).queryByRole('link', { name: 'Рекомендации по закупкам: Молочный суп' }) !== null).toBe(role !== 'order_operator')
    expect(within(noRecipe).queryByRole('link', { name: 'Добавить рецептуру: Без рецептуры' }) !== null).toBe(role === 'admin')
  })
  it('supports keyboard expansion, search, pagination reset and browser history', async () => {
    const { user, router } = mount('/stop-list?page=2'); await screen.findByText(/Недоступных блюд: 27. Страница 2/)
    await user.click(screen.getByRole('button', { name: 'Предыдущая страница' })); const soup = await card()
    const summary = within(soup).getByText('Состав и причины: Молочный суп'); summary.focus(); expect(document.activeElement).toBe(summary); await user.click(summary)
    expect(summary.closest('details')?.open).toBe(true)
    await user.type(screen.getByLabelText('Название блюда'), 'МОЛОЧНЫЙ{Enter}'); await screen.findByText(/Недоступных блюд: 1/)
    expect(screen.getAllByRole('article')).toHaveLength(1); expect(router.state.location.search).not.toContain('page=')
    await act(() => router.navigate(-1)); await screen.findByText(/Недоступных блюд: 27. Страница 1/)
    expect((screen.getByLabelText('Название блюда') as HTMLInputElement).value).toBe('')
  })
  it('isolates a delayed previous filter response from currentData', async () => {
    let release!: () => void
    const waiting = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('/api/v1/stop-list', async ({ request }) => {
      const search = new URL(request.url).searchParams.get('search') ?? '', items = stopListDishes().filter((dish) => dish.name.includes(search))
      if (search === 'Молочный') await waiting
      return Response.json({ items, total: items.length, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })
    }))
    const { router } = mount('/stop-list?search=Молочный'); await waitFor(() => expect(readCount()).toBe(1))
    await act(() => router.navigate('/stop-list?search=Десерт')); await card('Десерт без остатка')
    await act(async () => { release(); await waiting })
    expect(screen.queryByRole('article', { name: 'Блюдо: Молочный суп' })).toBeNull()
  })
  it('supports loading, stale data, first-load error and keyboard retry', async () => {
    setMockScenario('network-error'); const { store, user } = mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить стоп-лист' }); expect(screen.queryByRole('article')).toBeNull()
    setMockScenario('success'); screen.getByRole('button', { name: 'Попробовать снова' }).focus(); await user.keyboard('{Enter}'); await card()
    setMockScenario('slow'); store.dispatch(dishApi.util.invalidateTags(['StopList']))
    await waitFor(() => expect((screen.getByRole('button', { name: 'Обновить стоп-лист' }) as HTMLButtonElement).disabled).toBe(true)); expect(screen.queryByRole('status', { name: 'Загрузка стоп-листа' })).toBeNull(); expect((screen.getByRole('button', { name: 'Обновить стоп-лист' }) as HTMLButtonElement).disabled).toBe(true)
    await waitFor(() => expect((screen.getByRole('button', { name: 'Обновить стоп-лист' }) as HTMLButtonElement).disabled).toBe(false), { timeout: 3000 })
    setMockScenario('network-error'); store.dispatch(dishApi.util.invalidateTags(['StopList']))
    await screen.findByText(/Данные могут быть устаревшими/); expect(await card()).toBeTruthy()
  })
  it('suppresses cached data on access denial without clearing session', async () => {
    const { store } = mount(); await card(); setMockScenario('forbidden'); store.dispatch(dishApi.util.invalidateTags(['StopList']))
    await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByRole('article')).toBeNull(); expect(store.getState().session.status).toBe('authenticated')
  })
  it.each(['invalid-schema', 'invalid-json'] as const)('rejects %s without claiming availability', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить стоп-лист' }); expect(screen.queryByRole('heading', { name: 'Все блюда доступны' })).toBeNull()
  })
  it('distinguishes an empty list, no matches and an out-of-range page', async () => {
    setMockScenario('stop-list-empty'); const { router, user } = mount(); await screen.findByRole('heading', { name: 'Все блюда доступны' })
    setMockScenario('success'); await act(() => router.navigate('/stop-list?search=несуществующее')); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await act(() => router.navigate('/stop-list?page=3')); await screen.findByRole('heading', { name: 'На этой странице нет блюд' })
    await user.click(screen.getByRole('button', { name: 'К первой странице' })); await card()
  })
  it('refreshes after typed supply and order mutations', async () => {
    const { store } = mount('/stop-list?search=Молочный'); await card()
    const now = new Date().toISOString(), expires = new Date(Date.now() + 86400000).toISOString()
    await act(async () => { await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F12-TEST', received_at: now, items: [{ product_id: inventoryItems[0].product.id, quantity: '1.000', expires_at: expires }] } })).unwrap() })
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await act(async () => { await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body: { items: [{ dish_id: compositionDishes()[0].id, portions: 1, recipe_version: 1 }] } })).unwrap() })
    expect((await card()).textContent).toContain('99,001 мл'); expect(readCount()).toBe(3)
  })
  it('refreshes recipe changes, explicit tag invalidation', async () => {
    const { store } = mount('/stop-list?search=Молочный'); await card()
    setMockScenario('stop-list-recipe'); store.dispatch(dishApi.util.invalidateTags(['Dishes'])); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    setMockScenario('success'); store.dispatch(dishApi.util.invalidateTags(['StopList'])); await card()
    setMockScenario('stop-list-expired'); store.dispatch(dishApi.util.invalidateTags(['StopList']))
    await waitFor(() => expect(screen.getByRole('article').textContent).toContain('Остались только просроченные партии'))
  })
  it('refreshes expiration through central fallback without writes, querying only mounted reads', async () => {
    const { store } = mount('/stop-list?search=Молочный'); await card()
    await waitFor(() => expect(screen.queryByRole('status', { name: 'Загрузка стоп-листа' })).toBeNull())
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const live = createLiveEvents({ connect: () => ({ addEventListener: () => {}, close: () => {} }), visible: () => true, online: () => true, confirmSession: async () => true, refresh: () => { store.dispatch(dishApi.util.invalidateTags(['StopList'])) } })
    await act(async () => { store.dispatch(dishApi.util.invalidateTags(['StopList'])); await vi.advanceTimersByTimeAsync(20) })
    setMockScenario('stop-list-expired'); await act(async () => { await vi.advanceTimersByTimeAsync(30_020) }); vi.useRealTimers()
    await waitFor(() => expect(screen.getByRole('article').textContent).toContain('Остались только просроченные партии'))
    cleanup(); const count = readCount()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }); await act(() => vi.advanceTimersByTimeAsync(30_100)); vi.useRealTimers(); expect(readCount()).toBe(count)
    live.stop()
  })
  it('validates inactive/available rows, duplicate IDs and server time', () => {
    const items = stopListDishes().slice(0, 2), data = { items, total: 2, page: 1, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } }
    expect(stopListSchema.safeParse(data).success).toBe(true)
    expect(stopListSchema.safeParse({ ...data, items: [{ ...items[0], is_active: false }] }).success).toBe(false)
    expect(stopListSchema.safeParse({ ...data, items: [{ ...items[0], is_available: true }] }).success).toBe(false)
    expect(stopListSchema.safeParse({ ...data, items: [items[0], items[0]] }).success).toBe(false)
    expect(stopListSchema.safeParse({ ...data, meta: { server_time: 'wrong' } }).success).toBe(false)
  })
  it('rejects a mismatched pagination envelope', async () => {
    server.use(http.get('/api/v1/stop-list', () => Response.json({ items: [], total: 0, page: 2, page_size: 20, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить стоп-лист' }); expect(screen.queryByRole('heading', { name: 'Все блюда доступны' })).toBeNull()
  })
})
