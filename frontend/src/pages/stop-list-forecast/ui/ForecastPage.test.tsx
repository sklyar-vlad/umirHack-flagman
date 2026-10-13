// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { forecastApi, forecastRowSchema, forecastSearchError, forecastSearchParams, changeForecastSearch, defaultForecastSearch, readForecastSearch } from '@/entities/stop-list-forecast'
import { orderApi } from '@/entities/order/api/order-api'
import { handlers } from '@/app/mocks/handlers'
import { supplyApi } from '@/entities/supply/api/supply-api'
import { writeOffApi } from '@/entities/stock-movement/api/write-off-api'
import { productApi } from '@/entities/product'
import { dishApi } from '@/entities/dish'
import { resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { resetWriteOffFixtures } from '@/app/mocks/write-off-fixtures'
import { resetCatalogFixtures } from '@/app/mocks/catalog-fixtures'
import { resetDishEditorFixtures } from '@/app/mocks/dish-editor-fixtures'
import { forecastRows, forecastServerTime } from '@/app/mocks/forecast-fixtures'
import { resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { initialSessionState, sessionReducer, sections, sectionForPath, permittedReturnPath, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { AuthContext, type SessionController } from '@/features/auth'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { mockId } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { ForecastPage } from './ForecastPage'
import { assumptionLabel, forecastOutcome } from './forecast-labels'

const server = setupServer(...handlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [forecastApi.reducerPath]: forecastApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(800), name: 'Сотрудник', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Кладовая', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } }, middleware: (get) => get({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests' } } }).concat(forecastApi.middleware) })
}
function mount(path = '/analytics/stop-list-forecast', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const router = createMemoryRouter([{ path: '/analytics/stop-list-forecast', element: <RoleGate section={sections.find((s) => s.id === 'analytics')!}><ForecastPage /></RoleGate> }], { initialEntries: [path] })
  const auth: SessionController = { revalidate: vi.fn(async () => true), restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), hasAccess: () => true, getVersion: () => 0, registerSubscription: () => () => false }
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { router, user: userEvent.setup(), store }
}
const ready = () => screen.findByText('Активных блюд: 5. Страница 1. История: 7 дн. Горизонт: 72 ч.')
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); resetReceptionFixtures(); resetWriteOffFixtures(); resetCatalogFixtures(); resetDishEditorFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(forecastApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

const row = forecastRows(false)[0]
const endpoint = '/api/v1/analytics/stop-list-forecast'
const reads = () => requests.filter((r) => new URL(r.url).pathname === endpoint)
const envelope = () => ({ items: [row], total: 1, page: 1, page_size: 50, meta: { server_time: forecastServerTime, lookback_days: 7, horizon_hours: 72 } })

describe('F29 stop-list forecast', () => {
  it('accepts published causes, confidence, nullable dates and extensible safe assumptions', () => {
    for (const value of forecastRows(false)) expect(forecastRowSchema.safeParse(value).success).toBe(true)
    expect(forecastRowSchema.safeParse({ ...row, assumptions: ['future_model_assumption'] }).success).toBe(true)
    for (const patch of [{ cause: 'guaranteed' }, { confidence: 'high' }, { estimated_stop_at: '2026-02-30T12:00:00Z' }, { estimated_stop_at: '2026-10-13T09:00:00' }, { limiting_product_id: 'bad' }, { token: 'secret' }]) expect(forecastRowSchema.safeParse({ ...row, ...patch }).success).toBe(false)
  })
  it.each(['lookback_days=0', 'lookback_days=91', 'lookback_days=1.5', 'lookback_days=', 'lookback_days=1&lookback_days=2', 'horizon_hours=0', 'horizon_hours=721', 'horizon_hours=bad', 'page=0', 'page=9007199254740992', 'page_size=101'])('blocks invalid URL %s before requests', async (search) => {
    expect(forecastSearchError(readForecastSearch(new URLSearchParams(search)))).toBeTruthy()
    mount(`/analytics/stop-list-forecast?${search}`); await screen.findByRole('heading', { name: 'Некорректные настройки' }); expect(reads()).toHaveLength(0)
  })
  it('uses defaults and allowlists filters, safe login and page reset', () => {
    expect(readForecastSearch(new URLSearchParams())).toEqual(defaultForecastSearch)
    expect(sectionForPath('/analytics/stop-list-forecast')?.id).toBe('forecast')
    expect(sectionForPath('/forecast')?.id).toBe('forecast')
    expect(permittedReturnPath('manager', '/analytics/stop-list-forecast')).toBe('/analytics/stop-list-forecast')
    expect(permittedReturnPath('storekeeper', '/analytics/stop-list-forecast')).toBe('/')
    const values = readForecastSearch(new URLSearchParams('lookback_days=90&horizon_hours=720&page=3&page_size=1&sort=-dish_name&token=secret'))
    expect(forecastSearchError(values)).toBeNull(); expect([...forecastSearchParams(values).keys()]).toEqual(['lookback_days', 'horizon_hours', 'page', 'page_size'])
    expect(changeForecastSearch(values, { lookback_days: 1 }).page).toBe(1)
    expect(safeReturnUrl('/forecast?lookback_days=1&horizon_hours=720&page=2&token=secret&sort=bad')).toBe('/analytics/stop-list-forecast?lookback_days=1&horizon_hours=720&page=2')
    expect(safeReturnUrl('/analytics/stop-list-forecast?lookback_days=bad')).toBe('/analytics/stop-list-forecast')
  })
  it('shows server order, all causes, exact dates, confidence and distinct null outcomes without writes', async () => {
    mount(); await ready()
    const names = screen.getAllByRole('heading', { level: 2 }).map((node) => node.textContent)
    expect(names).toEqual(forecastRows(false).map((item) => item.dish_name))
    for (const label of ['Уже недоступно', 'Истечение срока партии', 'Ожидаемый расход', 'Недостаточно истории; по срокам остановки в горизонте не найдено', 'Остановка в пределах горизонта не ожидается']) expect(screen.getByText(label)).toBeTruthy()
    expect(screen.getAllByText('Дата не определена')).toHaveLength(2)
    expect(screen.getByText('13.10.2026, 12:30:00')).toBeTruthy()
    expect(document.querySelector('time[datetime="2026-10-13T09:45:00.000000001Z"]')?.getAttribute('title')).toBe('2026-10-13T09:45:00.000000001Z')
    expect(screen.getAllByText(/Отсутствие даты не означает/)).toHaveLength(2)
    expect(requests.every((r) => r.method === 'GET' && new URL(r.url).pathname === endpoint)).toBe(true)
    expect([...new URL(reads()[0].url).searchParams.keys()]).toEqual(['lookback_days', 'horizon_hours', 'page', 'page_size'])
    expect(screen.getAllByText(/Будущие поставки не учитываются/).length).toBeGreaterThan(0)
  })
  it('preserves unknown assumptions as plain text and handles absent recipes/empty assumptions', async () => {
    const item = { ...row, limiting_product_id: null, confidence: 'insufficient_data', assumptions: ['recipe_missing', '<img src=x onerror=alert(1)>'] }
    server.use(http.get(endpoint, () => Response.json({ ...envelope(), items: [item] })))
    const { user } = mount(); await screen.findByText('Активных блюд: 1. Страница 1. История: 7 дн. Горизонт: 72 ч.')
    await user.click(screen.getByText('Допущения модели')); expect(screen.getByText('У блюда нет рецептуры')).toBeTruthy()
    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeTruthy(); expect(document.querySelector('img')).toBeNull()
    expect(screen.getByText('Не определён')).toBeTruthy(); expect(screen.queryAllByRole('link', { name: 'Карточка продукта' })).toHaveLength(0)
    expect(assumptionLabel('unknown_model')).toBe('unknown_model')
    expect(forecastOutcome({ ...row, estimated_stop_at: null, assumptions: [], confidence: 'insufficient_data' })).toBe('Недостаточно истории для оценки спроса')
  })
  it('uses permitted admin detail links and manager recipe destination without admin product access', async () => {
    mount(); await ready()
    expect(screen.getAllByRole('link', { name: 'Рецептура блюда' })[0].getAttribute('href')).toBe(`/admin/dishes/${row.dish_id}/edit`)
    expect(screen.getAllByRole('link', { name: 'Карточка продукта' })[0].getAttribute('href')).toBe(`/admin/products/${row.limiting_product_id}/edit`)
  })
  it('allows managers and hides admin product destinations', async () => {
    mount('/analytics/stop-list-forecast', 'manager'); await ready()
    expect(screen.queryAllByRole('link', { name: 'Карточка продукта' })).toHaveLength(0)
    expect(screen.getAllByRole('link', { name: 'Рецептура блюда' })[0].getAttribute('href')).toBe(`/dishes/${row.dish_id}`)
  })
  it.each(['storekeeper', 'order_operator'] as const)('blocks %s before query mount', async (role) => {
    mount('/analytics/stop-list-forecast', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(reads()).toHaveLength(0)
  })
  it('validates form limits, applies boundary values, pages and restores URL on back', async () => {
    setMockScenario('forecast-demo'); const { user, router } = mount('/analytics/stop-list-forecast?page_size=20')
    await screen.findByText('Активных блюд: 27. Страница 1. История: 7 дн. Горизонт: 72 ч.')
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await screen.findByText('Активных блюд: 27. Страница 2. История: 7 дн. Горизонт: 72 ч.')
    const history = screen.getByLabelText('История расхода, дней'), horizon = screen.getByLabelText('Горизонт прогноза, часов')
    await user.clear(history); await user.type(history, '91'); await user.clear(horizon); await user.type(horizon, '721')
    await user.click(screen.getByRole('button', { name: 'Применить' })); expect(history.getAttribute('aria-invalid')).toBe('true'); expect(horizon.getAttribute('aria-invalid')).toBe('true')
    expect(new URLSearchParams(router.state.location.search).get('page')).toBe('2')
    await user.clear(history); await user.type(history, '1'); await user.clear(horizon); await user.type(horizon, '720'); await user.click(screen.getByRole('button', { name: 'Применить' }))
    await screen.findByText('Активных блюд: 27. Страница 1. История: 1 дн. Горизонт: 720 ч.')
    expect(new URLSearchParams(router.state.location.search).has('page')).toBe(false)
    await router.navigate(-1); await screen.findByText('Активных блюд: 27. Страница 2. История: 7 дн. Горизонт: 72 ч.')
    await user.click(screen.getByRole('button', { name: 'Сбросить настройки' })); await screen.findByText('Активных блюд: 27. Страница 1. История: 7 дн. Горизонт: 72 ч.'); expect(router.state.location.search).toBe('')
  })
  it('retains clearly stale data on same-argument errors, retries and suppresses it on 403', async () => {
    const { user } = mount(); await ready(); setMockScenario('forecast-error'); await user.click(screen.getByRole('button', { name: 'Обновить прогноз' })); await screen.findByText(/Данные могут быть устаревшими/); await ready()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await waitFor(() => expect(screen.queryByRole('heading', { name: 'Не удалось загрузить прогноз' })).toBeNull())
    setMockScenario('forecast-forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить прогноз' })); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByRole('list', { name: 'Оценки остановки блюд' })).toBeNull()
  })
  it('does not show previous arguments while new settings are loading', async () => {
    const { router } = mount(); await ready(); setMockScenario('forecast-slow'); await router.navigate('/analytics/stop-list-forecast?lookback_days=90')
    await screen.findByRole('status', { name: 'Загрузка прогноза' }); expect(screen.queryByRole('list', { name: 'Оценки остановки блюд' })).toBeNull()
  })
  it.each(['forecast-empty', 'forecast-invalid', 'forecast-error'] as const)('shows %s', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: scenario === 'forecast-empty' ? 'Активных блюд пока нет' : 'Не удалось загрузить прогноз' })
  })
  it.each(['duplicates', 'wrong-page', 'wrong-size', 'wrong-lookback', 'wrong-horizon', 'bad-date'] as const)('rejects %s before cache', async (kind) => {
    const data = envelope()
    if (kind === 'duplicates') { data.items = [row, row]; data.total = 2 }
    if (kind === 'wrong-page') data.page = 2
    if (kind === 'wrong-size') data.page_size = 20
    if (kind === 'wrong-lookback') data.meta.lookback_days = 8
    if (kind === 'wrong-horizon') data.meta.horizon_hours = 24
    if (kind === 'bad-date') data.meta.server_time = 'today'
    server.use(http.get(endpoint, () => Response.json(data))); const { store } = mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить прогноз' })
    expect(forecastApi.endpoints.stopListForecast.select(defaultForecastSearch)(store.getState()).data).toBeUndefined()
  })
  it('offers first page for an empty page without claiming missing history', async () => {
    const { user } = mount('/analytics/stop-list-forecast?page=99'); await screen.findByRole('heading', { name: 'На этой странице нет блюд' })
    await user.click(screen.getByRole('button', { name: 'К первой странице' })); await ready()
  })
  it('refreshes subscribed forecasts after stock/order/product/recipe writes but not checks', async () => {
    const { store } = mount(); await ready()
    const body = { items: [{ dish_id: mockId(800), portions: 1, recipe_version: 1 }] }
    const count = reads().length
    await store.dispatch(orderApi.endpoints.checkOrder.initiate(body)).unwrap(); expect(reads()).toHaveLength(count)
    const change = async (write: () => Promise<unknown>) => {
      const before = reads().length
      await write(); await waitFor(() => expect(reads().length).toBeGreaterThan(before))
      await waitFor(() => expect(forecastApi.endpoints.stopListForecast.select(defaultForecastSearch)(store.getState()).isLoading).toBe(false))
    }
    await change(async () => store.dispatch(orderApi.endpoints.createOrder.initiate({ key: crypto.randomUUID(), body })).unwrap())
    let batchId = ''
    await change(async () => {
      const supply = await store.dispatch(supplyApi.endpoints.receiveSupply.initiate({ key: crypto.randomUUID(), body: { number: 'F29-SUPPLY', received_at: new Date().toISOString(), items: [{ product_id: mockId(10), quantity: '100.000', expires_at: new Date(Date.now() + 86400000).toISOString() }] } })).unwrap()
      batchId = supply.items[0].batch.id
    })
    await change(async () => store.dispatch(writeOffApi.endpoints.writeOff.initiate({ key: crypto.randomUUID(), body: { batch_id: batchId, quantity: '1.000', reason: 'Порча' }, productId: mockId(10), unit: 'ml' })).unwrap())
    setMockScenario('role-admin')
    await change(async () => store.dispatch(productApi.endpoints.updateProduct.initiate({ id: mockId(10), body: { expected_revision: 1, name: 'Молоко F29' } })).unwrap())
    await change(async () => store.dispatch(dishApi.endpoints.updateDish.initiate({ id: mockId(800), body: { expected_revision: 1, name: 'Суп F29' }, previousVersion: 1 })).unwrap())
  })
})
