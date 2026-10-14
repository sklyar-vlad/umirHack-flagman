// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { dishApi, hotOffersListSchema, readHotOfferSearch, hotOfferSearchParams, changeHotOfferSearch } from '@/entities/dish'
import { initialSessionState, sessionReducer, replaceSession, saveWorkDraft, sections, type Role } from '@/entities/session'
import { orderApi } from '@/entities/order'
import { supplyApi } from '@/entities/supply'
import { addBasketDish } from '@/features/compose-order'
import { RoleGate } from '@/features/access'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { RecommendationsLanding } from '@/pages/procurement'
import { OrderComposer } from '@/widgets/order-composer'
import { hotOfferHandlers } from '@/app/mocks/hot-offer-handlers'
import { hotOfferItems, resetHotOfferFixtures } from '@/app/mocks/hot-offer-fixtures'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { receptionHandlers } from '@/app/mocks/reception-handlers'
import { resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { inventoryItems } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { HotOffersPage } from './HotOffersPage'

const server = setupServer(...hotOfferHandlers, ...orderCompositionHandlers, ...receptionHandlers), NativeRequest = globalThis.Request
const requests: { url: URL; method: string }[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [dishApi.reducerPath]: dishApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(dishApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/recommendations/offers', role: Role = 'manager', basket?: { dish: number; portions: number }) {
  const store = makeStore(role); stores.push(store)
  if (basket) store.dispatch(saveWorkDraft({ key: 'order', draft: { order: addBasketDish({ lines: [] }, compositionDishes()[basket.dish], basket.portions) } }))
  const router = createMemoryRouter([
    { path: '/recommendations/offers', element: <RoleGate section={sections.find((section) => section.id === 'offers')!}><HotOffersPage /></RoleGate> },
    { path: '/recommendations', element: <RecommendationsLanding /> },
    { path: '/orders/new', element: <OrderComposer onCreated={() => {}} /> },
  ], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const card = (name = 'Суп дня') => screen.findByRole('article', { name: `Предложение: ${name}` })
const reads = () => requests.filter(({ url }) => url.pathname === '/api/v1/hot-offers')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  HTMLDialogElement.prototype.showModal = function () { this.open = true }; HTMLDialogElement.prototype.close = function () { this.open = false }
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push({ url: new URL(request.url), method: request.method }))
})
beforeEach(() => { setMockScenario('success'); resetHotOfferFixtures(); resetCompositionFixtures(); resetReceptionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dishApi.util.resetApiState())); server.resetHandlers(); vi.useRealTimers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('published hot offers frontend with explicit MSW', () => {
  it('allowlists filters, uses 50 default and retains safe session returns', () => {
    expect(readHotOfferSearch(new URLSearchParams('page=0&page_size=101'))).toEqual({ search: '', page: 1, page_size: 50 })
    const state = readHotOfferSearch(new URLSearchParams('search=%25_%5C&page=2&page_size=1&sort=name'))
    expect(changeHotOfferSearch(state, { search: 'new' }).page).toBe(1)
    expect(hotOfferSearchParams(state).has('sort')).toBe(false)
    expect(safeReturnUrl(`/recommendations/offers?${hotOfferSearchParams(state)}&token=secret`)).toBe(`/recommendations/offers?${hotOfferSearchParams(state)}`)
    expect(safeReturnUrl('/recommendations?tab=hot-offers&search=Суп&password=secret')).toBe('/recommendations?search=%D0%A1%D1%83%D0%BF&tab=hot-offers')
  })
  it('preserves response order even for equal rounded scores; renders actual batch use and levels', async () => {
    const items = hotOfferItems().reverse()
    server.use(http.get('/api/v1/hot-offers', () => Response.json({ items, total: 2, page: 1, page_size: 50, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); const soup = await card()
    expect(screen.getAllByRole('article').map((node) => node.getAttribute('aria-label'))).toEqual(['Предложение: Салат дня', 'Предложение: Суп дня'])
    for (const text of ['Место: 1', '1,6667', 'Низкий приоритет', '15 порц.', '500 мл', '100 мл', 'ТЕСТ-101', 'строка 2']) expect(soup.textContent).toContain(text)
    expect((await card('Салат дня')).textContent).toContain('Средний приоритет')
    expect(screen.getByText(/не гарантируют возможность приготовить их вместе/)).toBeTruthy()
    expect(requests.some(({ method }) => method === 'POST')).toBe(false)
    expect(reads().every(({ url }) => !url.searchParams.has('sort'))).toBe(true)
  })
  it('shows high priority only when the server supplies it', async () => {
    setMockScenario('hot-offers-high'); mount(); expect((await card()).textContent).toContain('Высокий приоритет'); expect((await card('Салат дня')).textContent).not.toContain('Высокий приоритет')
  })
  it('preserves exact thousandths, large portion strings and a valid subsecond batch', async () => {
    const item = hotOfferItems()[0]; item.available_portions = '90071992547409931234'; item.problem_batches[0].remaining_quantity = '0.001'; item.problem_batches[0].used_for_one_portion = '0.001'; item.problem_batches[0].remaining_seconds = 0
    server.use(http.get('/api/v1/hot-offers', () => Response.json({ items: [item], total: 1, page: 1, page_size: 50, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); const soup = await card(); expect(soup.textContent).toContain('90071992547409931234'); expect(soup.textContent).toContain('0,001 мл'); expect(soup.textContent).not.toContain('0 с.')
  })
  it('redirects the tab entry preserving filters', async () => {
    const { router } = mount('/recommendations?tab=hot-offers&search=Суп&token=secret'); await card()
    expect(router.state.location.pathname).toBe('/recommendations/offers'); expect(router.state.location.search).not.toContain('token'); expect(screen.getAllByRole('article')).toHaveLength(1)
  })
  it.each(['/recommendations/offers', '/recommendations?tab=hot-offers'])('denies storekeeper before queries at %s', async (path) => {
    mount(path, 'storekeeper'); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it.each(['admin', 'manager', 'order_operator'] as const)('permits %s', async (role) => { mount(undefined, role); await card(); expect(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' })).toBeTruthy() })
  it('searches by keyboard, validates length, paginates with global rank and retains browser history', async () => {
    const { user, router } = mount('/recommendations/offers?page_size=1'); await card()
    await user.click(screen.getByRole('button', { name: 'Следующая страница' })); expect((await card('Салат дня')).textContent).toContain('Место: 2')
    await user.type(screen.getByLabelText('Поиск блюда'), 'СУП{Enter}'); await card(); expect(screen.getAllByRole('article')).toHaveLength(1); expect(router.state.location.search).not.toContain('page=')
    await act(() => router.navigate(-1)); await card('Салат дня')
    await user.clear(screen.getByLabelText('Поиск блюда')); await user.type(screen.getByLabelText('Поиск блюда'), 'a'.repeat(201) + '{Enter}'); await screen.findByText('Не больше 200 символов.')
  })
  it('isolates delayed responses from new filters', async () => {
    let release!: () => void; const waiting = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('/api/v1/hot-offers', async ({ request }) => {
      const search = new URL(request.url).searchParams.get('search') ?? '', items = hotOfferItems().filter((item) => item.dish.name.includes(search))
      if (search === 'Суп') await waiting
      return Response.json({ items, total: items.length, page: 1, page_size: 50, meta: { server_time: '2026-10-11T12:00:00Z' } })
    }))
    const { router } = mount('/recommendations/offers?search=Суп'); await waitFor(() => expect(reads()).toHaveLength(1))
    await act(() => router.navigate('/recommendations/offers?search=Салат')); await card('Салат дня')
    await act(async () => { release(); await waiting }); expect(screen.queryByRole('article', { name: 'Предложение: Суп дня' })).toBeNull()
  })
  it('reads composition by ID without changing basket; closing restores trigger focus', async () => {
    const { user, store } = mount(); await card(); const button = screen.getByRole('button', { name: 'Состав: Суп дня' }); button.focus(); await user.keyboard('{Enter}')
    const dialog = await screen.findByRole('dialog', { name: 'Состав: Суп дня' }); await within(dialog).findByText('Молоко — 100 мл на порцию')
    expect(requests.some(({ url }) => url.pathname === `/api/v1/dishes/${compositionDishes()[0].id}`)).toBe(true)
    expect(store.getState().session.basket).toEqual([]); expect(requests.some(({ method }) => method === 'POST')).toBe(false)
    await user.click(within(dialog).getByRole('button', { name: 'Закрыть' })); expect(document.activeElement).toBe(button)
  })
  it('handles dialog loading, retry and changed recipes', async () => {
    const { user } = mount(); await card(); setMockScenario('dish-detail-slow'); await user.click(screen.getByRole('button', { name: 'Состав: Суп дня' })); await screen.findByRole('status', { name: 'Загрузка состава' })
    await screen.findByText('Молоко — 100 мл на порцию', {}, { timeout: 3000 })
    await user.click(screen.getByRole('button', { name: 'Закрыть' })); setMockScenario('dish-detail-error'); await user.click(screen.getByRole('button', { name: 'Состав: Суп дня' })); await screen.findByRole('heading', { name: 'Не удалось загрузить состав' })
    setMockScenario('order-recipe-changed'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByText('Молоко — 150 мл на порцию'); await screen.findByText(/Рецептура изменилась после снимка/)
  })
  it.each(['dish-detail-missing', 'forbidden'] as const)('hides composition records for %s', async (scenario) => {
    const { user } = mount(); await card(); setMockScenario(scenario); await user.click(screen.getByRole('button', { name: 'Состав: Суп дня' }))
    await screen.findByRole('heading', { name: scenario === 'forbidden' ? 'Недостаточно прав' : 'Блюдо не найдено' }); expect(screen.queryByText('Молоко — 100 мл на порцию')).toBeNull()
  })
  it('adds exactly one portion to the existing basket and checks the combined ingredient shortage', async () => {
    const { user, router, store } = mount(undefined, 'order_operator', { dish: 0, portions: 15 }); await card('Салат дня')
    await user.click(screen.getByRole('button', { name: 'Добавить в заказ: Салат дня' })); await screen.findByText('Корзину нельзя оформить.')
    expect(router.state.location.pathname).toBe('/orders/new'); expect(store.getState().session.basket.map((line) => line.portions)).toEqual([15, 1])
    expect(requests.filter(({ url }) => url.pathname === '/api/v1/orders/check')).toHaveLength(1)
    expect(requests.some(({ url }) => url.pathname === '/api/v1/orders')).toBe(false)
    expect(screen.getByRole('button', { name: 'Оформить заказ' }).hasAttribute('disabled')).toBe(true)
  })
  it('increments the same dish without duplicating basket rows', async () => {
    const { user, store } = mount(undefined, 'manager', { dish: 0, portions: 2 }); await card(); await user.click(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' })); await screen.findByText('Корзина проверена: можно оформить.')
    expect(store.getState().session.basket.map((line) => line.portions)).toEqual([3])
  })
  it.each(['hot-offers-expired', 'order-recipe-changed', 'dish-detail-error', 'network-error'] as const)('preserves basket when adding fails due to %s', async (scenario) => {
    const { user, store, router } = mount(undefined, 'manager', { dish: 0, portions: 1 }); await card(); const before = store.getState().session.drafts.order
    setMockScenario(scenario); await user.click(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' })); await screen.findAllByRole('alert')
    expect(router.state.location.pathname).toBe('/recommendations/offers'); expect(store.getState().session.drafts.order).toEqual(before)
    expect(requests.some(({ method }) => method === 'POST')).toBe(false)
  })
  it('never transfers late addition to another account', async () => {
    let release!: () => void; const waiting = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('/api/v1/dishes/:id', async () => { await waiting; return Response.json(compositionDishes()[0]) }))
    const { user, store, router } = mount(); await card(); await user.click(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' }))
    await waitFor(() => expect(requests.some(({ url }) => url.pathname.includes('/dishes/'))).toBe(true))
    await act(() => { const state = store.getState().session; store.dispatch(replaceSession({ ...state, user: { ...state.user!, id: '22222222-2222-4222-8222-222222222222' } })) })
    await act(async () => { release(); await waiting }); await waitFor(() => expect(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' }).hasAttribute('disabled')).toBe(false))
    expect(store.getState().session.basket).toEqual([]); expect(router.state.location.pathname).toBe('/recommendations/offers')
  })
  it('does not redirect or add after the user changes list filters during an async read', async () => {
    let release!: () => void; const waiting = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('/api/v1/dishes/:id', async () => { await waiting; return Response.json(compositionDishes()[0]) }))
    const { user, store, router } = mount(); await card(); await user.click(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' }))
    await waitFor(() => expect(requests.some(({ url }) => url.pathname.includes('/dishes/'))).toBe(true))
    await act(() => router.navigate('/recommendations/offers?search=Салат')); await card('Салат дня')
    await act(async () => { release(); await waiting }); await waitFor(() => expect(screen.getByRole('button', { name: 'Добавить в заказ: Салат дня' }).hasAttribute('disabled')).toBe(false))
    expect(store.getState().session.basket).toEqual([]); expect(router.state.location.search).toContain('search=')
  })
  it('renders every affected batch independently including grams and integral pieces', async () => {
    const item = hotOfferItems()[0], batch = item.problem_batches[0]
    item.problem_batches.push({ ...batch, batch_id: '00000000-0000-4000-8000-000000009991', product_name: 'Мука', unit: 'g', line_number: 3, remaining_quantity: '123.456', used_for_one_portion: '0.001' }, { ...batch, batch_id: '00000000-0000-4000-8000-000000009992', product_name: 'Яйцо', unit: 'pcs', line_number: 4, remaining_quantity: '2.000', used_for_one_portion: '1.000' })
    server.use(http.get('/api/v1/hot-offers', () => Response.json({ items: [item], total: 1, page: 1, page_size: 50, meta: { server_time: '2026-10-11T12:00:00Z' } })))
    mount(); const soup = await card(); expect(within(soup).getAllByRole('listitem')).toHaveLength(3)
    for (const text of ['123,456 г', '0,001 г', '2 шт.', '1 шт.', 'строка 3', 'строка 4']) expect(soup.textContent).toContain(text)
  })
  it('locks addition for an unresolved attempt', async () => {
    const { store } = mount(); await card(); const dish = compositionDishes()[0]
    await act(() => store.dispatch(saveWorkDraft({ key: 'order', draft: { order: { ...addBasketDish({ lines: [] }, dish), attempt: { key: crypto.randomUUID(), body: { items: [{ dish_id: dish.id, portions: 1, recipe_version: 1 }] }, startedAt: Date.now() } } } })))
    expect(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' }).hasAttribute('disabled')).toBe(true); await screen.findByText(/Корзина заблокирована/)
  })
  it('offers keyboard retry, disables stale additions and suppresses cached 403', async () => {
    setMockScenario('network-error'); const { user, store } = mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить предложения' })
    setMockScenario('success'); screen.getByRole('button', { name: 'Попробовать снова' }).focus(); await user.keyboard('{Enter}'); await card()
    setMockScenario('network-error'); store.dispatch(dishApi.util.invalidateTags(['HotOffers'])); await screen.findByText(/Данные могут быть устаревшими/); expect(await card()).toBeTruthy(); expect(screen.getByRole('button', { name: 'Добавить в заказ: Суп дня' }).hasAttribute('disabled')).toBe(true)
    setMockScenario('forbidden'); store.dispatch(dishApi.util.invalidateTags(['HotOffers'])); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByRole('article')).toBeNull(); expect(store.getState().session.status).toBe('authenticated')
  })
  it('explains empty/no-match/out-of-range and recovers to page one', async () => {
    setMockScenario('hot-offers-empty'); const { router, user } = mount(); await screen.findByRole('heading', { name: 'Нет блюд для приоритетной продажи' })
    setMockScenario('success'); await act(() => router.navigate('/recommendations/offers?search=%25_%5C')); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    await act(() => router.navigate('/recommendations/offers?page=9')); await screen.findByRole('heading', { name: 'На этой странице нет предложений' }); await user.click(screen.getByRole('button', { name: 'К первой странице' })); await card()
  })
  it('refreshes after confirmed orders and reception, then removes exhausted problem batches', async () => {
    const { store } = mount(); await card(); const dish = compositionDishes()[0]
    await act(async () => { await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body: { items: [{ dish_id: dish.id, portions: 1, recipe_version: 1 }] } })).unwrap() })
    await waitFor(() => expect(screen.getByRole('article', { name: 'Предложение: Суп дня' }).textContent).toContain('400 мл'))
    await act(async () => { await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F14-TEST', received_at: new Date().toISOString(), items: [{ product_id: inventoryItems[0].product.id, quantity: '100.000', expires_at: new Date(Date.now() + 86400000).toISOString() }] } })).unwrap() })
    await waitFor(() => expect(reads()).toHaveLength(3))
    await act(async () => { await store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body: { items: [{ dish_id: dish.id, portions: 4, recipe_version: 1 }] } })).unwrap() })
    await screen.findByRole('heading', { name: 'Нет блюд для приоритетной продажи' })
  })
  it('refreshes external recipe changes and expiration on explicit tag invalidation', async () => {
    const { store } = mount(); await card(); setMockScenario('order-recipe-changed'); store.dispatch(dishApi.util.invalidateTags(['HotOffers'])); await waitFor(() => expect(screen.getByRole('article', { name: 'Предложение: Суп дня' }).textContent).toContain('Рецептура: 2'))
    setMockScenario('hot-offers-expired'); store.dispatch(dishApi.util.invalidateTags(['HotOffers'])); await screen.findByRole('heading', { name: 'Нет блюд для приоритетной продажи' })
  })
  it.each(['invalid-json', 'invalid-schema'] as const)('rejects %s without claiming an empty list', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить предложения' }); expect(screen.queryByRole('heading', { name: 'Нет блюд для приоритетной продажи' })).toBeNull()
  })
  it('rejects unpublished units, fractional pieces, zero use, duplicate dishes, invalid scores and pagination', async () => {
    const items = hotOfferItems(), data = { items, total: 2, page: 1, page_size: 50, meta: { server_time: '2026-10-11T12:00:00Z' } }
    expect(hotOffersListSchema.safeParse(data).success).toBe(true)
    for (const patch of [{ priority_score: '100.0001' }, { priority_level: 'urgent' }, { available_portions: '0' }, { problem_batches: [] }]) expect(hotOffersListSchema.safeParse({ ...data, items: [{ ...items[0], ...patch }] }).success).toBe(false)
    for (const patch of [{ used_for_one_portion: '0.000' }, { unit: 'kg' }, { unit: 'pcs', used_for_one_portion: '1.001' }]) expect(hotOffersListSchema.safeParse({ ...data, items: [{ ...items[0], problem_batches: [{ ...items[0].problem_batches[0], ...patch }] }] }).success).toBe(false)
    expect(hotOffersListSchema.safeParse({ ...data, items: [items[0], items[0]] }).success).toBe(false)
    server.use(http.get('/api/v1/hot-offers', () => Response.json({ ...data, page: 2 }))); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить предложения' })
  })
})
