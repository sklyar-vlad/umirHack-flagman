// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { MemoryRouter, Route, Routes } from 'react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { initialSessionState, sessionReducer, type Role } from '@/entities/session'
import { invalidateSupplies, supplyApi, supplyDetailSchema } from '@/entities/supply'
import { supplyHandlers } from '@/app/mocks/supply-handlers'
import { supplyFixtures } from '@/app/mocks/supply-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { SuppliesPage } from './SuppliesPage'
import { InventoryPage } from '@/pages/inventory'
import { inventoryHandlers } from '@/app/mocks/inventory-handlers'
import { supplyDetailFixture } from '@/app/mocks/supply-detail-fixtures'
import { inventorySelectionParams } from '@/entities/inventory'
import { safeReturnUrl } from '@/features/auth'
import { SupplyDetailPage } from './SupplyDetailPage'
import { SupplyEntryPage } from './SupplyEntryPage'

const server = setupServer(...supplyHandlers, ...inventoryHandlers)
const NativeRequest = globalThis.Request, requests: URL[] = []
function makeStore(role: Role = 'manager') {
  return configureStore({
    reducer: { session: sessionReducer, [supplyApi.reducerPath]: supplyApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: supplyFixtures[0].created_by.id, name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(supplyApi.middleware),
  })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = `/supplies/${supplyFixtures[0].id}`, role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  render(<Provider store={store}><MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/inventory" element={<InventoryPage />} />
    <Route path="/supplies" element={<SuppliesPage />} />
    <Route path="/supplies/new" element={<SupplyEntryPage />} />
    <Route path="/supplies/:id" element={<SupplyDetailPage />} />
  </Routes></MemoryRouter></Provider>)
  return { store, user: userEvent.setup() }
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) }
  })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', ({ request }) => requests.push(new URL(request.url)))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(supplyApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
const fixture = supplyDetailFixture(supplyFixtures[0])
const table = () => screen.findByRole('table', { name: 'Состав поставки' })

describe('published SupplyDetail contract', () => {
  it('accepts original separate repeated products and depleted lines', () => {
    expect(supplyDetailSchema.safeParse(fixture).success).toBe(true)
    expect(fixture.items.filter((line) => line.product.name === 'Молоко')).toHaveLength(23)
    expect(fixture.items.some((line) => line.batch.status === 'depleted')).toBe(true)
  })
  it.each([
    { items_count: 26 }, { items: [] }, { meta: {} },
    { items: [fixture.items[0], ...fixture.items.slice(0, 26)] },
    { items: [{ ...fixture.items[0], product: { ...fixture.items[0].product, unit: 'g' } }, ...fixture.items.slice(1)] },
    { items: [{ ...fixture.items[0], batch: { ...fixture.items[0].batch, product_id: supplyFixtures[0].id } }, ...fixture.items.slice(1)] },
    { items: [{ ...fixture.items[0], batch: { ...fixture.items[0].batch, supply: { ...fixture.items[0].batch.supply, id: supplyFixtures[1].id } } }, ...fixture.items.slice(1)] },
    { items: [{ ...fixture.items[0], batch: { ...fixture.items[0].batch, remaining_quantity: '1.0001' } }, ...fixture.items.slice(1)] },
  ])('rejects incomplete/inconsistent details: %j', (patch) => {
    expect(supplyDetailSchema.safeParse({ ...fixture, ...patch }).success).toBe(false)
  })
})

describe('supply detail in explicit MSW mode', () => {
  it('loads id without filters and renders server order, quantities, dates and immutable actions', async () => {
    mount()
    const rows = within(await table()).getAllByRole('row').slice(1)
    expect(rows).toHaveLength(27)
    expect(rows[0].textContent).toContain('Молоко')
    expect(rows[0].textContent).toContain('0,2 л')
    expect(rows[1].textContent).toContain('0,5 л')
    expect(rows[22].textContent).toContain('Израсходована')
    expect(rows[22].textContent).toContain('0 л')
    expect(rows[23].textContent).toContain('Творог — только просрочка')
    expect(rows[24].textContent).toContain('99999999,999999 кг')
    expect(rows[26].textContent).toContain('10 шт')
    rows.forEach((row, index) => expect(row.textContent).toContain(fixture.items[index].batch.id))
    expect(screen.getByText('10.10.2026, 12:05')).toBeTruthy()
    expect(screen.getByText('10.10.2026, 12:00')).toBeTruthy()
    expect(screen.getByText(/Остатки и статусы на 11.10.2026, 15:00/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Редактировать|Удалить|Сохранить|Изменить/ })).toBeNull()
    expect(requests[0].pathname).toBe(`/api/v1/supplies/${fixture.id}`)
    expect(requests[0].search).toBe('')
  })
  it('uses snapshot order without client sorting or grouping', async () => {
    server.use(http.get('/api/v1/supplies/:id', () => Response.json({ ...fixture, items: [...fixture.items].reverse() })))
    mount()
    expect(within(await table()).getAllByRole('row')[1].textContent).toContain('Яйцо')
  })
  it('shows loading, retains only this snapshot on network error and supports keyboard retry', async () => {
    setMockScenario('slow')
    const { store, user } = mount()
    expect(screen.getByRole('status', { name: 'Загрузка поставки' })).toBeTruthy()
    await screen.findByRole('table', { name: 'Состав поставки' }, { timeout: 4000 })
    setMockScenario('network-error'); store.dispatch(invalidateSupplies())
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставку' })
    expect(screen.getByText(/Данные могут быть устаревшими/)).toBeTruthy()
    expect(await table()).toBeTruthy()
    setMockScenario('success')
    screen.getByRole('button', { name: 'Попробовать снова' }).focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Не удалось загрузить поставку' })).toBeNull())
  })
  it.each(['supply-not-found', 'forbidden'] as const)('hides cached details on %s', async (scenario) => {
    const { store } = mount()
    await table()
    setMockScenario(scenario); store.dispatch(invalidateSupplies())
    await screen.findByRole('heading', { name: scenario === 'forbidden' ? 'Недостаточно прав' : 'Поставка не найдена' })
    expect(screen.queryByRole('table')).toBeNull()
    expect(screen.queryByRole('heading', { name: 'Поставка ТЕСТ-101' })).toBeNull()
    expect(store.getState().session.status).toBe('authenticated')
  })
  it('shows first-load error and retries without inventing details', async () => {
    setMockScenario('server-error')
    const { user } = mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставку' })
    expect(screen.queryByRole('table')).toBeNull()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await table()
  })
  it('rejects malformed path before issuing requests', async () => {
    mount('/supplies/not-an-id')
    await screen.findByRole('heading', { name: 'Некорректный адрес поставки' })
    expect(requests).toHaveLength(0)
  })
  it('shows resource 404 for a valid missing id, with safe history return', async () => {
    mount(`/supplies/${supplyFixtures[0].created_by.id}?returnTo=https://evil.test`)
    await screen.findByRole('heading', { name: 'Поставка не найдена' })
    expect(screen.getByRole('link', { name: 'К истории поставок' }).getAttribute('href')).toBe('/supplies')
  })
  it('rejects a successful response for another supply', async () => {
    server.use(http.get('/api/v1/supplies/:id', () => Response.json(supplyDetailFixture(supplyFixtures[1]))))
    mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставку' })
    expect(screen.queryByRole('table')).toBeNull()
  })
  it('never shows the previous supply while another id is loading', async () => {
    const { user } = mount()
    await table()
    await user.click(screen.getByRole('link', { name: 'К истории поставок' }))
    const history = await screen.findByRole('table', { name: 'История поставок' })
    setMockScenario('slow')
    await user.click(within(history).getByRole('link', { name: 'Поставка ГРАНИЦА-С' }))
    expect(screen.getByRole('status', { name: 'Загрузка поставки' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: 'Поставка ТЕСТ-101' })).toBeNull()
    expect(screen.queryByRole('table')).toBeNull()
    await screen.findByRole('heading', { name: 'Поставка ГРАНИЦА-С' }, { timeout: 4000 })
    expect(within(await table()).getAllByRole('row')).toHaveLength(2)
  })
  it('opens exact product/supply full history including depleted and preserves history filters', async () => {
    const { user } = mount(`/supplies/${fixture.id}?returnTo=${encodeURIComponent('/supplies?search=ТЕСТ&page_size=20')}`)
    const grid = await table()
    expect(screen.getByRole('link', { name: 'К истории поставок' }).getAttribute('href')).toBe('/supplies?search=%D0%A2%D0%95%D0%A1%D0%A2&page_size=20')
    const link = within(grid).getByRole('link', { name: 'На склад: Молоко, строка 23' })
    link.focus(); await user.keyboard('{Enter}')
    await screen.findByRole('heading', { name: 'Партии выбранного продукта' })
    const panel = within(screen.getByRole('region', { name: 'Выбор из поставки' }))
    await panel.findByText('Израсходована')
    expect((panel.getByLabelText('Показать партии') as HTMLSelectElement).value).toBe('all')
    expect(panel.getAllByText('Выбранная поставка').length).toBeGreaterThan(0)
    const batchRequest = requests.find((url) => url.pathname.endsWith('/batches'))!
    expect(batchRequest.searchParams.get('status')).toBe('all')
    expect(batchRequest.searchParams.has('supply_id')).toBe(false)
    expect(requests.filter((url) => url.pathname === '/api/v1/inventory').every((url) => !url.searchParams.has('product_id') && !url.searchParams.has('supply_id'))).toBe(true)
    await user.click(panel.getByRole('button', { name: 'Снять выбор поставки' }))
    expect(screen.queryByRole('region', { name: 'Выбор из поставки' })).toBeNull()
  })
  it('keeps depleted product history accessible when stock search is empty', async () => {
    setMockScenario('inventory-empty')
    mount(`/inventory?product_id=${fixture.items[22].product.id}&supply_id=${fixture.id}`)
    await screen.findByText('Израсходована')
    expect(screen.getByRole('region', { name: 'Выбор из поставки' })).toBeTruthy()
  })
  it('sanitizes UI-only selection in login returns', () => {
    const url = `/inventory?search=Молоко&product_id=${fixture.items[0].product.id}&supply_id=${fixture.id}&password=secret`
    const safe = new URL(safeReturnUrl(url), 'https://internal.test')
    expect(safe.searchParams.has('password')).toBe(false)
    expect(safe.searchParams.get('product_id')).toBe(fixture.items[0].product.id)
    expect(inventorySelectionParams(new URLSearchParams('product_id=bad&supply_id=bad')).size).toBe(0)
  })
})
