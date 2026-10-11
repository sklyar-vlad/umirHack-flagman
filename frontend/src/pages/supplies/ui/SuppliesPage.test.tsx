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
import { defaultSupplySearch, invalidateSupplies, supplyApi, supplyListSchema, supplySummarySchema } from '@/entities/supply'
import { supplyHandlers } from '@/app/mocks/supply-handlers'
import { newlyReceivedSupply, supplyFixtures } from '@/app/mocks/supply-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { SuppliesPage } from './SuppliesPage'
import { SupplyEntryPage } from './SupplyEntryPage'

const server = setupServer(...supplyHandlers)
const NativeRequest = globalThis.Request, requests: URL[] = []
function makeStore(role: Role = 'manager') {
  return configureStore({
    reducer: { session: sessionReducer, [supplyApi.reducerPath]: supplyApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: supplyFixtures[0].created_by.id, name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(supplyApi.middleware),
  })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/supplies?search=ТЕСТ', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  render(<Provider store={store}><MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/supplies" element={<SuppliesPage />} />
    <Route path="/supplies/new" element={<SupplyEntryPage reception />} />
    <Route path="/supplies/:id" element={<SupplyEntryPage />} />
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
const table = () => screen.findByRole('table', { name: 'История поставок' })
const body = { items: supplyFixtures, total: supplyFixtures.length, page: 1, page_size: 50, meta: {} }

describe('published supply contracts', () => {
  it('accepts received status, fractional instants, all original lines and empty meta', () => {
    expect(supplyListSchema.safeParse(body).success).toBe(true)
    expect(supplyListSchema.safeParse({ ...body, items: [], total: 26, page: 3 }).success).toBe(true)
  })
  it.each([{ items_count: 0 }, { items_count: 101 }, { items_count: 1.5 }, { status: 'pending' }, { id: 'bad' }, { created_at: '2026-02-30T12:00:00Z' }, { received_at: null }, { created_by: { id: 'bad', name: 'Test' } }])('rejects invalid summary: %j', (patch) => {
    expect(supplySummarySchema.safeParse({ ...supplyFixtures[0], ...patch }).success).toBe(false)
  })
})

describe('supply history in explicit MSW mode', () => {
  it('shows distinct created/reception dates, original line count, author and status', async () => {
    mount()
    const row = within(await table()).getAllByRole('row')[1]
    expect(row.textContent).toContain('10.10.2026, 12:05:00')
    expect(row.textContent).toContain('10.10.2026, 12:00:00')
    expect(row.textContent).toContain('27')
    expect(row.textContent).toContain('Тестовый кладовщик')
    expect(row.textContent).toContain('Принята')
    expect(requests[0].searchParams.get('sort')).toBe('-created_at')
  })
  it.each(['admin', 'manager', 'storekeeper'] as const)('permits the reception entry for %s', async (role) => {
    mount('/supplies?search=ТЕСТ', role)
    await table()
    expect(screen.getByRole('link', { name: 'Новая поставка' })).toBeTruthy()
  })
  it('hides reception action from an operator even when the page is mounted without its route guard', async () => {
    mount('/supplies', 'order_operator')
    await table()
    expect(screen.queryByRole('link', { name: 'Новая поставка' })).toBeNull()
  })
  it('uses server total and stable created_at DESC/id DESC pagination', async () => {
    const { user } = mount('/supplies?page_size=20')
    expect(within(await table()).getAllByRole('row')).toHaveLength(21)
    expect(screen.getByText('Найдено поставок: 26. Страница 1.')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await screen.findByText('Найдено поставок: 26. Страница 2.')
    expect(within(await table()).getAllByRole('row')).toHaveLength(7)
    const ties = within(await table()).getAllByRole('row').slice(1).map((row) => row.textContent)
    expect(ties[0]).toContain('ИСТОРИЯ-6')
    await user.type(screen.getByLabelText('Поиск по номеру'), 'ТЕСТ{Enter}')
    await screen.findByText('Найдено поставок: 1. Страница 1.')
    expect(requests.at(-1)?.searchParams.get('page')).toBe('1')
    expect(requests.at(-1)?.searchParams.get('page_size')).toBe('20')
  })
  it('implements created_at lower inclusion/upper exclusion instead of received_at', async () => {
    mount('/supplies?from=2026-10-09T21%3A00%3A00Z&to=2026-10-10T21%3A00%3A00Z')
    const list = await table()
    expect(within(list).getByRole('link', { name: 'Поставка ГРАНИЦА-С' })).toBeTruthy()
    expect(within(list).queryByRole('link', { name: 'Поставка ГРАНИЦА-ДО' })).toBeNull()
    expect(screen.getByText('Найдено поставок: 2. Страница 1.')).toBeTruthy()
    expect((screen.getByLabelText('Создана с (включительно)') as HTMLInputElement).value).toBe('2026-10-10T00:00')
  })
  it('validates inverted local dates with field association and makes no extra request', async () => {
    const { user } = mount()
    await table()
    await user.type(screen.getByLabelText('Создана с (включительно)'), '2026-10-11T00:00')
    await user.type(screen.getByLabelText('Создана до (не включая)'), '2026-10-10T00:00')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByText('Начало периода должно быть не позже конца.')
    expect(screen.getByLabelText('Создана до (не включая)').getAttribute('aria-invalid')).toBe('true')
    expect(requests).toHaveLength(1)
  })
  it('does not request invalid URL periods and recovers with reset', async () => {
    const { user } = mount('/supplies?from=not-a-date')
    expect(screen.getByRole('heading', { name: 'Некорректный период' })).toBeTruthy()
    expect(requests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
    await table()
  })
  it('searches percent, underscore and backslash as literal characters', async () => {
    mount('/supplies?search=%25_%5C')
    expect(within(await table()).getAllByRole('row')).toHaveLength(2)
    expect(within(await table()).getByRole('link', { name: 'Поставка Буквальный %_\\' })).toBeTruthy()
  })
  it('distinguishes empty history, no results and an out-of-range page', async () => {
    setMockScenario('supplies-empty')
    const { store, user } = mount('/supplies')
    await screen.findByRole('heading', { name: 'Поставок пока нет' })
    setMockScenario('success'); store.dispatch(invalidateSupplies())
    await table()
    await user.type(screen.getByLabelText('Поиск по номеру'), 'не существует{Enter}')
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
  })
  it('recovers an out-of-range page without losing the server total', async () => {
    const { user } = mount('/supplies?page=3&page_size=20')
    await screen.findByRole('heading', { name: 'На этой странице нет поставок' })
    expect(screen.getByText('Найдено поставок: 26. Страница 3.')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'К первой странице' }))
    await table()
    expect(screen.getByText('Найдено поставок: 26. Страница 1.')).toBeTruthy()
  })
  it('retains the current successful list on a network error, retries, and never shows it for changed filters', async () => {
    const { store, user } = mount()
    await table()
    setMockScenario('network-error'); store.dispatch(invalidateSupplies())
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставки' })
    expect(within(await table()).getByRole('link', { name: 'Поставка ТЕСТ-101' })).toBeTruthy()
    expect(screen.getByText(/Данные могут быть устаревшими/)).toBeTruthy()
    await user.type(screen.getByLabelText('Поиск по номеру'), '-другой{Enter}')
    await waitFor(() => expect(screen.queryByRole('table')).toBeNull())
    await screen.findByRole('button', { name: 'Попробовать снова' })
    setMockScenario('success')
    await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
  })
  it('shows first-load errors without inventing an empty history, retries and suppresses cached data on 403', async () => {
    setMockScenario('server-error')
    const { store, user } = mount()
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставки' })
    expect(screen.queryByRole('heading', { name: 'Поставок пока нет' })).toBeNull()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await table()
    setMockScenario('forbidden'); store.dispatch(invalidateSupplies())
    await screen.findByRole('heading', { name: 'Недостаточно прав' })
    expect(screen.queryByRole('table')).toBeNull()
    expect(store.getState().session.status).toBe('authenticated')
  })
  it.each([
    { ...body, page: 2 }, { ...body, page_size: 20 }, { ...body, total: 0 },
    { ...body, items: [supplyFixtures[0], supplyFixtures[0]] }, { ...body, items: [{ ...supplyFixtures[0], items_count: 0 }] },
  ])('rejects mismatched or malformed server envelopes before rendering', async (data) => {
    server.use(http.get('/api/v1/supplies', () => Response.json(data)))
    mount('/supplies')
    await screen.findByRole('heading', { name: 'Не удалось загрузить поставки' })
    expect(screen.queryByRole('table')).toBeNull()
  })
  it('refreshes on return from details and reception, retaining filters and receiving invalidations', async () => {
    const { user, store } = mount('/supplies?page_size=20')
    await user.click(within(await table()).getByRole('link', { name: 'Поставка ТЕСТ-101' }))
    await screen.findByRole('heading', { name: 'Детали поставки' })
    const count = requests.length
    await user.click(screen.getByRole('link', { name: 'К истории поставок' }))
    await table()
    await waitFor(() => expect(requests.length).toBeGreaterThan(count))
    expect(requests.at(-1)?.searchParams.get('page_size')).toBe('20')
    await user.click(screen.getByRole('link', { name: 'Новая поставка' }))
    await screen.findByRole('heading', { name: 'Новая поставка' })
    setMockScenario('supplies-received')
    await user.click(screen.getByRole('link', { name: 'К истории поставок' }))
    await waitFor(() => expect(within(screen.getByRole('table')).getByRole('link', { name: `Поставка ${newlyReceivedSupply.number}` })).toBeTruthy())
    setMockScenario('success'); store.dispatch(invalidateSupplies())
    await waitFor(() => expect(within(screen.getByRole('table')).queryByRole('link', { name: `Поставка ${newlyReceivedSupply.number}` })).toBeNull())
    expect(supplyApi.endpoints.supplies.select({ ...defaultSupplySearch, page_size: 20 })(store.getState()).data?.total).toBe(26)
  })
})
