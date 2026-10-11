// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { MemoryRouter } from 'react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { initialSessionState, sessionReducer, type Role } from '@/entities/session'
import { inventoryApi, inventoryResponseSchema, batchesResponseSchema, categoriesResponseSchema } from '@/entities/inventory'
import { inventoryHandlers } from '@/app/mocks/inventory-handlers'
import { inventoryItems, inventoryBatches, inventoryCategories, inventoryServerTime, mockId } from '@/app/mocks/inventory-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { InventoryPage } from './InventoryPage'
import { remainingTime } from './inventory-format'

const server = setupServer(...inventoryHandlers)
const NativeRequest = globalThis.Request
const requests: URL[] = []
const stores: ReturnType<typeof makeStore>[] = []
function makeStore(role: Role = 'manager') {
  return configureStore({
    reducer: { session: sessionReducer, [inventoryApi.reducerPath]: inventoryApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(99), name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(inventoryApi.middleware),
  })
}
function mount(path = '/inventory?search=Молоко', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  render(<Provider store={store}><MemoryRouter initialEntries={[path]}><InventoryPage /></MemoryRouter></Provider>)
  return userEvent.setup()
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) }
  })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(inventoryApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
async function milk() { return screen.findByRole('article', { name: 'Молоко' }) }

describe('published warehouse contracts', () => {
  const body = { items: inventoryItems, total: inventoryItems.length, page: 1, page_size: 50, meta: { server_time: inventoryServerTime }, counts: { all: 25, expired: 2, expiring: 1, out_of_stock: 22, deficit: 2 }, attention_batches: [], attention_total: 3 }
  it('accepts exact aggregates over numeric(14,3), explicit null and published batch/category fields', () => {
    expect(inventoryResponseSchema.safeParse(body).success).toBe(true)
    expect(batchesResponseSchema.safeParse({ ...body, items: inventoryBatches }).success).toBe(true)
    expect(categoriesResponseSchema.safeParse({ ...body, items: inventoryCategories, meta: {} }).success).toBe(true)
  })
  it.each([123, '1e3', '1.0001', '-1.000'])('rejects a malformed stock quantity: %s', (raw) => {
    expect(inventoryResponseSchema.safeParse({ ...body, items: [{ ...inventoryItems[0], available_quantity: raw }] }).success).toBe(false)
  })
  it('rejects incompatible display units and fractional pieces before rendering', () => {
    expect(inventoryResponseSchema.safeParse({ ...body, items: [{ ...inventoryItems[0], product: { ...inventoryItems[0].product, display_unit: 'kg' } }] }).success).toBe(false)
    expect(inventoryResponseSchema.safeParse({ ...body, items: [{ ...inventoryItems[4], available_quantity: '1.500' }] }).success).toBe(false)
  })
  it('shows signed server seconds, including expiry equality, independent of the local clock', () => {
    expect(remainingTime(64800)).toBe('До срока: 18 ч')
    expect(remainingTime(0, true)).toBe('Срок истёк 0 с назад')
    expect(remainingTime(-1)).toBe('Срок истёк 1 с назад')
    expect(remainingTime(null)).toBe('Нет годных партий')
  })
})

describe('inventory screen against explicit MSW', () => {
  it('loads list without fetching batches; keyboard expansion shows separate same-supply lines in FEFO order', async () => {
    const user = mount()
    const card = await milk()
    expect(within(card).getByText('1,5 л')).toBeTruthy()
    expect(within(card).getByText('1,7 л')).toBeTruthy()
    expect(within(card).getByText('22')).toBeTruthy()
    expect(requests.filter((url) => url.pathname.endsWith('/batches'))).toHaveLength(0)
    within(card).getByRole('button', { name: 'Раскрыть партии Молоко' }).focus()
    await user.keyboard('{Enter}')
    const panel = await screen.findByRole('region', { name: 'Партии: Молоко' })
    await within(panel).findByText('ТЕСТ-101 · строка 1')
    const rows = within(panel).getAllByRole('listitem')
    expect(rows[0].textContent).toContain('строка 1')
    expect(rows[1].textContent).toContain('строка 2')
    expect(rows[0].textContent).toContain('0,2 л')
    expect(rows[1].textContent).toContain('0,5 л')
    expect(within(panel).getByText('Срок истёк 1 ч назад')).toBeTruthy()
    expect(requests.find((url) => url.pathname.endsWith('/batches'))?.searchParams.has('status')).toBe(false)
    await user.selectOptions(within(panel).getByLabelText('Показать партии'), 'depleted')
    await within(panel).findByText('Израсходована')
    expect(within(panel).queryByRole('link', { name: /Списать партию/ })).toBeNull()
    expect(requests.at(-1)?.searchParams.get('status')).toBe('depleted')
  })
  it('clearly distinguishes expired-only stock and null expiry/target from zero', async () => {
    mount('/inventory?search=Творог')
    const card = await screen.findByRole('article', { name: 'Творог — только просрочка' })
    expect(within(card).getAllByText('0,3 кг')).toHaveLength(2)
    expect(within(card).getByText('Годный остаток').nextElementSibling?.textContent).toBe('0 кг')
    expect(within(card).getByText('Нет годных партий')).toBeTruthy()
    expect(within(card).getByText('Не задан')).toBeTruthy()
    expect(within(card).getByText('Нет годного остатка')).toBeTruthy()
    expect(card.textContent).toContain('Продукт отключён')
  })
  it('uses server overlapping counters rather than the current-page length', async () => {
    mount('/inventory?page_size=1')
    await screen.findByRole('article')
    expect(screen.getByRole('button', { name: 'Все · 25' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'С просрочкой · 2' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Нет годного остатка · 22' })).toBeTruthy()
  })
  it.each([['all', 25], ['expiring', 1], ['expired', 2], ['out_of_stock', 22], ['deficit', 2]] as const)('requests the published %s filter and renders its server result', async (filter, count) => {
    mount(`/inventory?filter=${filter}`)
    await waitFor(() => expect(screen.getAllByRole('article')).toHaveLength(count))
    expect(requests.find((url) => url.pathname.endsWith('/inventory'))?.searchParams.get('filter')).toBe(filter)
  })
  it('distinguishes a page beyond total from an empty warehouse and recovers', async () => {
    const user = mount('/inventory?page=3&page_size=20')
    await screen.findByRole('heading', { name: 'На этой странице нет продуктов' })
    expect(screen.queryByRole('heading', { name: 'Склад пока пуст' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'К первой странице' }))
    await waitFor(() => expect(screen.getAllByRole('article')).toHaveLength(20))
  })
  it('resets API page when filtering and retains literal search in typed requests', async () => {
    const user = mount('/inventory?page=2&page_size=20')
    await screen.findByRole('button', { name: 'Все · 25' })
    await user.click(screen.getByRole('button', { name: 'С просрочкой · 2' }))
    await milk()
    const filterRequest = requests.filter((url) => url.pathname.endsWith('/inventory')).at(-1)!
    expect(filterRequest.searchParams.get('page')).toBe('1')
    expect(filterRequest.searchParams.get('filter')).toBe('expired')
    await user.type(screen.getByLabelText('Поиск продукта'), '%_\\')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
    expect(requests.filter((url) => url.pathname.endsWith('/inventory')).at(-1)?.searchParams.get('search')).toBe('%_\\')
  })
  it('keeps product list accessible when one panel fails and recovers by retry', async () => {
    const user = mount(); await milk()
    setMockScenario('batches-error')
    await user.click(screen.getByRole('button', { name: 'Раскрыть партии Молоко' }))
    await screen.findByRole('heading', { name: 'Не удалось загрузить партии' })
    expect(screen.getByRole('article', { name: 'Молоко' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Не удалось загрузить склад' })).toBeNull()
    setMockScenario('success')
    await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await screen.findByText('ТЕСТ-101 · строка 1')
  })
  it('renders independent empty panel while retaining the product', async () => {
    setMockScenario('batches-empty'); const user = mount(); await milk()
    await user.click(screen.getByRole('button', { name: 'Раскрыть партии Молоко' }))
    await screen.findByRole('heading', { name: 'Нет подходящих партий' })
    expect(screen.getByRole('article', { name: 'Молоко' })).toBeTruthy()
  })
  it.each(['admin', 'manager', 'storekeeper', 'order_operator'] as const)('shows permission-based supply/writeoff actions for %s', async (role) => {
    const user = mount(undefined, role); await milk()
    await user.click(screen.getByRole('button', { name: 'Раскрыть партии Молоко' }))
    await screen.findByText('ТЕСТ-101 · строка 1')
    const link = screen.queryByRole('link', { name: 'К поставке ТЕСТ-101, строка 1' })
    if (role === 'order_operator') { expect(link).toBeNull(); expect(screen.queryAllByRole('link', { name: /Списать партию/ })).toHaveLength(0) }
    else {
      expect(link?.getAttribute('href')).toBe(`/supplies/${mockId(100)}`)
      expect(screen.getByRole('link', { name: 'Списать партию · строка 1' }).getAttribute('href')).toContain(`batch_id=${mockId(201)}`)
    }
  })
  it('never renders another product batches from a malformed backend response', async () => {
    server.use(http.get('/api/v1/products/:id/batches', () => Response.json({ items: [inventoryBatches[0]], total: 1, page: 1, page_size: 20, meta: { server_time: inventoryServerTime } })))
    const user = mount('/inventory?search=Творог')
    await screen.findByRole('article')
    await user.click(screen.getByRole('button', { name: 'Раскрыть партии Творог — только просрочка' }))
    await screen.findByRole('heading', { name: 'Не удалось загрузить партии' })
    expect(screen.queryByText('ТЕСТ-101 · строка 1')).toBeNull()
  })
  it('collects category pages beyond 100 without silent truncation', async () => {
    const categories = Array.from({ length: 101 }, (_, index) => ({ id: mockId(500 + index), name: `Категория ${index}`, revision: 1 }))
    server.use(http.get('/api/v1/product-categories', ({ request }) => {
      const page = Number(new URL(request.url).searchParams.get('page'))
      return Response.json({ items: categories.slice((page - 1) * 100, page * 100), total: 101, page, page_size: 100, meta: {} })
    }))
    mount(); await milk()
    await waitFor(() => expect(screen.getByRole('option', { name: 'Категория 100' })).toBeTruthy())
    expect(requests.filter((url) => url.pathname.endsWith('/product-categories')).map((url) => url.searchParams.get('page'))).toEqual(['1', '2'])
  })
  it.each(['server-error', 'invalid-schema', 'network-error'])('shows a safe list error for %s', async (scenario) => {
    setMockScenario(scenario); mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить склад' })
    expect(screen.queryByRole('article')).toBeNull()
  })
  it('handles server 403 without clearing the session or disclosing stock', async () => {
    setMockScenario('forbidden'); mount()
    await screen.findAllByRole('heading', { name: 'Недостаточно прав' })
    expect(stores[0].getState().session.status).toBe('authenticated')
    expect(screen.queryByRole('article')).toBeNull()
  })
})
