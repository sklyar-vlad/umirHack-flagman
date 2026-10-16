import { dishApi, dishChangesReducer } from '@/entities/dish'
// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { delay } from 'msw'
import { initialSessionState, sessionReducer, saveWorkDraft } from '@/entities/session'
import { orderApi, orderDetailSchema, orderCheckSchema } from '@/entities/order'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { compositionCheck, compositionDishes, composedOrders, composedAttempts, resetCompositionFixtures, changeCompositionRecipe } from '@/app/mocks/order-composition-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { basketDishSchema, type OrderRequest } from '@/shared/api/order-work'
import { createMockError } from '@/app/mocks/responses'
import { OrderComposer } from './OrderComposer'

const server = setupServer(...orderCompositionHandlers), NativeRequest = globalThis.Request
const stores: ReturnType<typeof makeStore>[] = [], requests: { path: string; method: string; key: string | null; body: string }[] = []
function makeStore() {
  return configureStore({ reducer: { session: sessionReducer, dishChanges: dishChangesReducer, [orderApi.reducerPath]: orderApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, draftOwner: '11111111-1111-4111-8111-111111111111', user: { id: '11111111-1111-4111-8111-111111111111', role: 'manager' as const, name: 'Тест', email: 'test@example.test' } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(orderApi.middleware) })
}
function mount(store = makeStore()) {
  stores.push(store)
  const created = vi.fn()
  const router = createMemoryRouter([{ path: '/orders/new', element: <OrderComposer onCreated={created} /> }], { initialEntries: ['/orders/new'] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, created, user: userEvent.setup() }
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', async ({ request }) => requests.push({ path: new URL(request.url).pathname, method: request.method, key: request.headers.get('Idempotency-Key'), body: request.method === 'POST' ? await request.clone().text() : '' }))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(orderApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
const add = async (user: ReturnType<typeof userEvent.setup>, name = 'Суп дня') => user.click(await screen.findByRole('button', { name: `Добавить ${name}` }))
const checked = async () => waitFor(() => expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(false))
const posts = () => requests.filter((r) => r.path === '/api/v1/orders' && r.method === 'POST')

describe('order composer in explicit MSW test mode', () => {
  it('F20 recipe writes invalidate a checked mounted basket and require recheck after recipe review', async () => {
    const { store, user } = mount(); await add(user); await checked()
    const dish = compositionDishes()[0]
    server.use(http.patch('/api/v1/dishes/:id', async ({ request }) => {
      const body = await request.json() as { name?: string; recipe?: unknown }
      if (body.recipe) { changeCompositionRecipe(); return Response.json(compositionDishes()[0]) }
      return Response.json({ ...dish, name: body.name, revision: 2 })
    }))
    await act(async () => { await store.dispatch(dishApi.endpoints.updateDish.initiate({ id: dish.id, body: { expected_revision: 1, name: 'Суп обновлён' }, previousVersion: 1 })) })
    await checked(); expect(requests.filter((request) => request.path.endsWith('/orders/check'))).toHaveLength(2)
    await act(async () => { await store.dispatch(dishApi.endpoints.updateDish.initiate({ id: dish.id, body: { expected_revision: 1, recipe: [{ product_id: dish.recipe[0].product_id, quantity_per_portion: '150.000' }] }, previousVersion: 1 })) })
    await screen.findByRole('heading', { name: 'Подтвердите актуальный состав' }); expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(true)
    await waitFor(() => expect((screen.getByRole('button', { name: 'Подтверждаю актуальный состав' }) as HTMLButtonElement).disabled).toBe(false))
    await user.click(screen.getByRole('button', { name: 'Подтверждаю актуальный состав' })); await checked()
    expect(requests.filter((request) => request.path.endsWith('/orders/check')).length).toBeGreaterThanOrEqual(3)
    expect(store.getState().session.drafts.order.order?.lines[0].dish.recipe_version).toBe(2)
    expect(posts()).toHaveLength(0)
  })
  it('merges repeated additions, validates portions and uses server total rather than independent availability', async () => {
    const { user, store } = mount()
    await add(user); await add(user)
    expect((screen.getByLabelText(/^Порции: Суп дня/) as HTMLInputElement).value).toBe('2')
    const input = screen.getByLabelText(/^Порции: Суп дня/)
    await user.clear(input); await user.type(input, '0')
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(true)
    await user.clear(input); await user.type(input, '8'); await add(user, 'Салат дня')
    await user.clear(screen.getByLabelText(/^Порции: Салат дня/)); await user.type(screen.getByLabelText(/^Порции: Салат дня/), '8')
    await screen.findByText('Корзину нельзя оформить.')
    const table = screen.getByRole('table', { name: 'Продукты для всей корзины' })
    expect(table.textContent).toContain('1600 мл'); expect(table.textContent).toContain('1500 мл'); expect(table.textContent).toContain('100 мл')
    expect(store.getState().session.basket).toHaveLength(2)
    await user.click(screen.getByRole('button', { name: 'Удалить Салат дня' })); await checked()
  })
  it('debounces and ignores old out-of-order checks; no idempotency key on read-only check', async () => {
    server.use(http.post('/api/v1/orders/check', async ({ request }) => {
      const body = await request.json() as OrderRequest
      if (body.items[0].portions === 1) await delay(900)
      return Response.json(compositionCheck(body))
    }))
    const { user } = mount(); await add(user)
    await waitFor(() => expect(requests.some((r) => r.path.endsWith('/orders/check'))).toBe(true))
    await user.type(screen.getByLabelText(/^Порции: Суп дня/), '6')
    await screen.findByText('Корзину нельзя оформить.')
    await new Promise((resolve) => setTimeout(resolve, 1000))
    expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(true)
    expect(requests.filter((r) => r.path.endsWith('/orders/check')).every((r) => r.key === null)).toBe(true)
    expect(requests.filter((r) => r.path.endsWith('/orders/check'))).toHaveLength(2)
    expect(composedAttempts()).toHaveLength(0)
  })
  it('blocks duplicate submit, clears only on confirmed matching success and invalidates reads', async () => {
    setMockScenario('order-submit-slow')
    const { user, store, created } = mount(); await add(user); await checked()
    await user.dblClick(screen.getByRole('button', { name: 'Оформить заказ' }))
    expect(store.getState().session.drafts.order.order?.attempt?.body.items[0].recipe_version).toBe(1)
    expect(store.getState().session.basket).toHaveLength(1)
    await waitFor(() => expect(created).toHaveBeenCalledTimes(1), { timeout: 3000 })
    expect(posts()).toHaveLength(1)
    expect(store.getState().session.basket).toEqual([]); expect(store.getState().session.drafts.order).toBeUndefined()
    expect(orderDetailSchema.safeParse(created.mock.calls[0][0]).success).toBe(true)
    expect(created.mock.calls[0][0].items[0].allocations[0].batch_id).toBe('00000000-0000-4000-8000-000000000202')
    await waitFor(() => expect(requests.filter((r) => r.path === '/api/v1/dishes')).toHaveLength(2))
  })
  it('recovers a lost success with identical body/key after remount and never creates twice', async () => {
    setMockScenario('order-lost-response')
    const first = mount(); await add(first.user); await checked(); await first.user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await screen.findByText(/Результат отправки неизвестен. Корзина сохранена и заблокирована/)
    expect(first.created).not.toHaveBeenCalled(); expect(composedOrders()).toHaveLength(1)
    const saved = first.store.getState().session.drafts.order.order?.attempt
    expect((screen.getByLabelText(/^Порции: Суп дня/) as HTMLInputElement).disabled).toBe(true)
    cleanup(); setMockScenario('success'); const second = mount(first.store)
    await second.user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' }))
    await waitFor(() => expect(second.created).toHaveBeenCalledTimes(1))
    expect(posts()).toHaveLength(2); expect(posts()[0].body).toBe(posts()[1].body); expect(posts()[0].key).toBe(saved?.key); expect(posts()[1].key).toBe(saved?.key)
    expect(composedOrders()).toHaveLength(1)
  })
  it('retains invalid success as uncertain instead of clearing the basket', async () => {
    server.use(http.post('/api/v1/orders', () => Response.json({ id: crypto.randomUUID(), status: 'completed' }, { status: 201 })))
    const { user, store } = mount(); await add(user); await checked(); await user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await screen.findByText(/Результат отправки неизвестен. Корзина сохранена и заблокирована/)
    expect(store.getState().session.drafts.order.order?.attempt).toBeTruthy(); expect(store.getState().session.basket).toHaveLength(1)
  })
  it.each(['order-shortage', 'order-validation'])('keeps basket on definitive %s and requires a fresh check/new attempt', async (scenario) => {
    setMockScenario(scenario)
    const { user, store } = mount(); await add(user); await checked(); await user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await screen.findByText(scenario === 'order-shortage' ? /Сохранённая попытка:/ : /Порции отклонены сервером/)
    expect(store.getState().session.drafts.order.order?.attempt).toBeUndefined(); expect(store.getState().session.basket).toHaveLength(1)
    expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(true)
    const first = posts()[0].key
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Проверить ещё раз' })); await checked(); await user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await waitFor(() => expect(posts()).toHaveLength(2)); expect(posts()[1].key).not.toBe(first)
  })
  it.each(['order-final-recipe', 'order-final-inactive'])('loads new composition and requires explicit confirmation after %s', async (scenario) => {
    setMockScenario(scenario)
    const { user } = mount(); await add(user); await checked(); await user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await screen.findByRole('heading', { name: 'Подтвердите актуальный состав' })
    const panel = screen.getByRole('region', { name: 'Изменение рецептуры' })
    await within(panel).findByText(scenario === 'order-final-recipe' ? /150 мл/ : 'Блюдо отключено')
    expect((screen.getByRole('button', { name: 'Оформить заказ' }) as HTMLButtonElement).disabled).toBe(true)
    expect(requests.filter((r) => r.path.endsWith('/orders/check'))).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Подтверждаю актуальный состав' }))
    if (scenario === 'order-final-inactive') await screen.findByText('Проверьте целые порции, активность и рецептуры блюд.')
    else { setMockScenario('order-recipe-changed'); await checked() }
  })
  it('loads current recipe on a check mismatch without recording an order attempt', async () => {
    const { user } = mount(); await add(user); await checked()
    setMockScenario('order-recipe-changed'); await user.click(screen.getByRole('button', { name: 'Проверить ещё раз' }))
    await screen.findByRole('heading', { name: 'Подтвердите актуальный состав' })
    await within(screen.getByRole('region', { name: 'Изменение рецептуры' })).findByText(/150 мл/)
    expect(composedAttempts()).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Подтверждаю актуальный состав' })); await checked()
  })
  it('keeps 429 attempt and accepts only exact replay after retry delay', async () => {
    setMockScenario('order-rate-limit')
    const { user, created, store } = mount(); await add(user); await checked(); await user.click(screen.getByRole('button', { name: 'Оформить заказ' }))
    await screen.findByText(/Лимит запросов: подождите 1 с/)
    const key = store.getState().session.drafts.order.order?.attempt?.key
    await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); expect(posts()).toHaveLength(1)
    await new Promise((resolve) => setTimeout(resolve, 1050)); setMockScenario('success')
    await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await waitFor(() => expect(created).toHaveBeenCalledTimes(1))
    expect(posts()[1].key).toBe(key)
  })
  it('rejects attempts beyond safe replay retention and leaves basket intact', async () => {
    const store = makeStore(), dish = basketDishSchema.parse(compositionDishes()[0])
    store.dispatch(saveWorkDraft({ key: 'order', draft: { order: { lines: [{ dish, portions: '1' }], attempt: { key: crypto.randomUUID(), startedAt: 0, body: { items: [{ dish_id: dish.id, portions: 1, recipe_version: 1 }] } } } } }))
    const { user } = mount(store); await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByText(/Срок безопасного повтора истёк/); expect(posts()).toHaveLength(0)
  })
  it('accepts published merged duplicate requests and rejects a successful response for another basket version', async () => {
    const { store } = mount(), dish = compositionDishes()[0]
    server.use(http.post('/api/v1/orders/check', () => Response.json(compositionCheck({ items: [{ dish_id: dish.id, portions: 2, recipe_version: 1 }] }))))
    const response = await store.dispatch(orderApi.endpoints.checkOrder.initiate({ items: [{ dish_id: dish.id, portions: 1 }, { dish_id: dish.id, portions: 1, recipe_version: 1 }] }))
    expect(response.data?.items[0].portions).toBe(2)
    server.use(http.post('/api/v1/orders/check', () => Response.json(compositionCheck({ items: [{ dish_id: dish.id, portions: 1, recipe_version: 1 }] }))))
    const invalid = await store.dispatch(orderApi.endpoints.checkOrder.initiate({ items: [{ dish_id: dish.id, portions: 2, recipe_version: 1 }] }))
    expect(invalid.error).toMatchObject({ code: 'INVALID_RESPONSE' })
    expect(posts()).toHaveLength(0)
  })
  it('retries check network errors without saved business refusals and rejects dishonest can_create responses', async () => {
    server.use(http.post('/api/v1/orders/check', () => createMockError(500, 'INTERNAL_ERROR', 'Ошибка')))
    const { user } = mount(); await add(user); await screen.findByRole('heading', { name: 'Не удалось проверить корзину' })
    expect(composedAttempts()).toHaveLength(0); expect(posts()).toHaveLength(0)
    server.resetHandlers(); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await checked()
    const invalid = { ...compositionCheck({ items: [{ dish_id: compositionDishes()[0].id, portions: 20, recipe_version: 1 }] }), can_create: true }
    expect(orderCheckSchema.safeParse(invalid).success).toBe(false)
  })
})
