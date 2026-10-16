// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider, Link } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { initialSessionState, replaceSession, sessionReducer, type Role, type SessionState } from '@/entities/session'
import { baseApi } from '@/shared/api'
import { defaultInventorySearch, inventoryApi } from '@/entities/inventory'
import { supplyApi } from '@/entities/supply'
import { supplyHandlers } from '@/app/mocks/supply-handlers'
import { writeOffApi, writeOffTags } from '@/entities/stock-movement'
import type { WriteOffDraft, WriteOffRequest } from '@/shared/api/write-off-work'
import { inventoryBatches, inventoryItems, mockId } from '@/app/mocks/inventory-fixtures'
import { inventoryHandlers } from '@/app/mocks/inventory-handlers'
import { writeOffHandlers } from '@/app/mocks/write-off-handlers'
import { resetWriteOffFixtures, writtenOffMovements } from '@/app/mocks/write-off-fixtures'
import { resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { acceptReception, resetReceptionFixtures } from '@/app/mocks/reception-fixtures'
import { setMockScenario, type MockScenario } from '@/app/mocks/scenarios'
import { createMockError } from '@/app/mocks/responses'
import { WriteOffWorkspace } from './WriteOffWorkspace'
import { useWriteOff } from './write-off-context'

const server = setupServer(...writeOffHandlers, ...inventoryHandlers, ...supplyHandlers)
const NativeRequest = globalThis.Request
const requests: { key: string | null; body: WriteOffRequest; csrf: string | null }[] = []
const stores: ReturnType<typeof makeStore>[] = []
const initial: WriteOffDraft = { batch: inventoryBatches[0], product: inventoryItems[0].product, values: { quantity: '', reason: '' } }
function makeStore(draft?: WriteOffDraft, role: Role = 'manager') {
  const session: SessionState = { ...initialSessionState, status: 'authenticated', user: { id: mockId(900), name: 'Тест', email: 'test@example.test', role }, draftOwner: mockId(900), drafts: draft ? { writeOff: { writeOff: draft } } : {}, settings: { timezone: 'Europe/Moscow', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } }
  return configureStore({ reducer: { session: sessionReducer, [baseApi.reducerPath]: supplyApi.reducer }, preloadedState: { session }, middleware: (getDefault) => getDefault({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests' } } }).concat(supplyApi.middleware) })
}
function Trigger() {
  const open = useWriteOff()
  return <><button onClick={(event) => open(initial, event.currentTarget)}>Списать выбранную партию</button><Link to="/elsewhere">Уйти со склада</Link></>
}
function mount(draft?: WriteOffDraft, role?: Role) {
  const store = makeStore(draft, role); stores.push(store)
  const router = createMemoryRouter([{ path: '/inventory', element: <WriteOffWorkspace><Trigger /></WriteOffWorkspace> }, { path: '/elsewhere', element: <h1>Другая страница</h1> }, { path: '/stock-movements', element: <h1>История движений</h1> }], { initialEntries: ['/inventory'] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
async function open(user: ReturnType<typeof userEvent.setup>) { await user.click(screen.getByRole('button', { name: 'Списать выбранную партию' })) }
async function fill(user: ReturnType<typeof userEvent.setup>, quantity = '0.001125', reason = 'Порча упаковки') {
  await user.type(screen.getByLabelText(/^Количество списания/), quantity)
  await user.type(screen.getByLabelText(/^Причина списания/), reason)
}
async function confirm(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Списать' }))
  await screen.findByRole('dialog', { name: 'Подтвердить списание?' })
  await user.click(screen.getByRole('button', { name: 'Подтвердить списание' }))
}
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', async ({ request }) => { if (request.method === 'POST') requests.push({ key: request.headers.get('Idempotency-Key'), body: await request.clone().json() as WriteOffRequest, csrf: request.headers.get('X-CSRF-Token') }) })
})
beforeEach(() => { requests.length = 0; resetWriteOffFixtures(); resetCompositionFixtures(); resetReceptionFixtures(); setMockScenario('success') })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(baseApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('manual batch write-off against explicit MSW', () => {
  it('shows the exact selected batch, waits for confirmation, submits base units with CSRF and displays only confirmed StockMovement', async () => {
    const { user, store } = mount(); await open(user)
    expect(screen.getByText('ТЕСТ-101 · строка 1')).toBeTruthy(); expect(screen.getByText(/Физический остаток: 0,2 л/)).toBeTruthy()
    expect(screen.getByText(/Просрочена/)).toBeTruthy()
    await fill(user); await user.click(screen.getByRole('button', { name: 'Списать' }))
    expect(requests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Вернуться к форме' })); expect(requests).toHaveLength(0)
    await confirm(user); await screen.findByRole('dialog', { name: 'Списание выполнено' })
    expect(requests[0].body).toEqual({ batch_id: initial.batch.id, quantity: '1.125', reason: 'Порча упаковки' }); expect(requests[0].csrf).toBe('mock-csrf-for-local-tests')
    expect(writtenOffMovements()).toHaveLength(1); expect(store.getState().session.drafts.writeOff).toBeUndefined()
    expect(screen.getByText(/198,875 мл/)).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Готово' })); expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Списать выбранную партию' }))
  })
  it('writes off a newly received batch and refetches its exact physical/available projection without reintroducing depleted receipt stock', async () => {
    const supply = acceptReception(crypto.randomUUID(), { number: 'ПРИЁМКА-F25', received_at: '2026-10-13T00:00:00Z', items: [{ product_id: initial.product.id, quantity: '100.000', expires_at: '2099-01-01T00:00:00Z' }] })
    const line = supply.items[0]
    const { user, store } = mount({ batch: line.batch, product: line.product, values: { quantity: '', reason: '' } })
    const subscription = store.dispatch(supplyApi.endpoints.supply.initiate(supply.id)); await subscription.unwrap()
    await fill(user, '0.1'); await confirm(user); await screen.findByRole('dialog', { name: 'Списание выполнено' })
    const data = await store.dispatch(inventoryApi.endpoints.inventory.initiate({ ...defaultInventorySearch, search: initial.product.name })).unwrap()
    const stock = data.items.find((item) => item.product.id === initial.product.id)!
    expect(stock.physical_quantity).toBe('1700.000'); expect(stock.available_quantity).toBe('1500.000'); expect(stock.batches_count).toBe(22)
    await waitFor(() => expect(supplyApi.endpoints.supply.select(supply.id)(store.getState()).data?.items[0].batch.remaining_quantity).toBe('0.000')); subscription.unsubscribe()
    expect(writtenOffMovements()).toHaveLength(1)
  })
  it('rejects empty, zero and over-stock input without any POST, keeps dirty close confirmation', async () => {
    const { user } = mount(); await open(user)
    await user.click(screen.getByRole('button', { name: 'Списать' })); await screen.findByText(/Укажите причину: 1–500/)
    await fill(user, '0.200001'); await user.click(screen.getByRole('button', { name: 'Списать' })); await screen.findByText(/превышает показанный/)
    expect(requests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Закрыть списание' })); await screen.findByRole('dialog', { name: 'Закрыть без сохранения?' })
    await user.click(screen.getByRole('button', { name: 'Остаться' })); expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.200001')
    await user.click(screen.getByRole('button', { name: 'Закрыть списание' })); await user.click(screen.getByRole('button', { name: 'Отбросить черновик' }))
    expect(screen.queryByRole('dialog', { name: 'Списать из партии' })).toBeNull()
  })
  it.each(['writeoff-network', 'writeoff-lost-response', 'writeoff-in-progress', 'writeoff-invalid'] as const)('retains exact UUID/body and performs one explicit replay after %s', async (scenario) => {
    setMockScenario(scenario); const { user, store } = mount(); await open(user); await fill(user); await confirm(user)
    await screen.findByText(/Результат отправки неизвестен/)
    expect(screen.getByLabelText(/^Количество списания/).matches(':disabled')).toBe(true)
    expect(store.getState().session.drafts.writeOff.writeOff?.attempt?.key).toBe(requests[0].key)
    expect(requests).toHaveLength(1)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByRole('dialog', { name: 'Списание выполнено' })
    expect(requests).toHaveLength(2); expect(requests[1]).toEqual(requests[0]); expect(writtenOffMovements()).toHaveLength(1)
  })
  it('preserves unknown request when closing and reopens the original batch instead of another selection', async () => {
    setMockScenario('writeoff-lost-response'); const { user } = mount(); await open(user); await fill(user); await confirm(user)
    await screen.findByText(/Результат отправки неизвестен/)
    await user.click(screen.getByRole('button', { name: 'Закрыть списание' })); await screen.findByRole('button', { name: 'Продолжить списание' })
    await user.click(screen.getByRole('button', { name: 'Продолжить списание' })); expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.001125')
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByRole('dialog', { name: 'Списание выполнено' })
    expect(requests[1]).toEqual(requests[0])
  })
  it('preserves quantity/reason after a competing order, rereads the batch and uses a new key for the corrected request', async () => {
    setMockScenario('writeoff-stock-changed'); const { user } = mount({ ...initial, batch: inventoryBatches[1] }); await fill(user, '0.4'); await confirm(user)
    await screen.findByText(/Остаток партии изменился/)
    expect(screen.getByLabelText(/^Причина списания/)).toHaveProperty('value', 'Порча упаковки')
    await user.click(screen.getByRole('button', { name: 'Перечитать остаток' })); await screen.findByText(/Физический остаток: 0,25 л/)
    expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.4')
    await user.clear(screen.getByLabelText(/^Количество списания/)); await user.type(screen.getByLabelText(/^Количество списания/), '0.25'); await confirm(user)
    await screen.findByRole('dialog', { name: 'Списание выполнено' }); expect(requests[1].key).not.toBe(requests[0].key)
    expect(writtenOffMovements().filter((item) => item.type === 'manual_writeoff')).toHaveLength(1)
  })
  it('keeps stock conflict form during a failed fresh read, then retries reading safely', async () => {
    setMockScenario('writeoff-stock-changed'); const { user } = mount({ ...initial, batch: inventoryBatches[1] }); await fill(user, '0.4'); await confirm(user); await screen.findByText(/Остаток партии изменился/)
    setMockScenario('batches-error'); await user.click(screen.getByRole('button', { name: 'Перечитать остаток' })); await screen.findByText(/Не удалось перечитать остаток/)
    expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.4')
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Перечитать остаток' })); await screen.findByText(/Физический остаток: 0,25 л/)
  })
  it('rejects an incompatible batch unit during stock conflict reload while preserving the form', async () => {
    setMockScenario('writeoff-stock-changed'); const { user } = mount({ ...initial, batch: inventoryBatches[1] }); await fill(user, '0.4'); await confirm(user); await screen.findByText(/Остаток партии изменился/)
    server.use(http.get('/api/v1/products/:id/batches', () => Response.json({ items: [{ ...inventoryBatches[1], unit: 'g' }], total: 1, page: 1, page_size: 100, meta: { server_time: '2026-10-13T00:00:00Z' } })))
    await user.click(screen.getByRole('button', { name: 'Перечитать остаток' })); await screen.findByText(/Не удалось перечитать остаток/)
    expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.4'); expect(screen.getByText(/Физический остаток: 0,5 л/)).toBeTruthy()
  })
  it('maps only allowlisted 422 fields, restores focus and permits correction with a new key', async () => {
    setMockScenario('writeoff-validation'); const { user } = mount(); await open(user); await fill(user); await confirm(user)
    await screen.findByText('Сервер отклонил это значение. Проверьте поле.')
    await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText(/^Причина списания/)))
    expect(screen.queryByText('Не показывать')).toBeNull()
    setMockScenario('success'); await user.type(screen.getByLabelText(/^Причина списания/), ' исправлено'); await confirm(user)
    await screen.findByRole('dialog', { name: 'Списание выполнено' }); expect(requests[1].key).not.toBe(requests[0].key)
  })
  it.each([403, 429] as const)('retains unresolved replay on early access/rate rejection %s', async (status) => {
    const attempt = { key: crypto.randomUUID(), startedAt: Date.now(), body: { batch_id: initial.batch.id, quantity: '1.125', reason: 'Порча' } }
    server.use(http.post('/api/v1/write-offs', () => createMockError(status, 'ACCESS_OR_RATE', 'Ранний отказ')))
    const { user, store } = mount({ ...initial, values: { quantity: '0.001125', reason: 'Порча' }, attempt })
    await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByText(/Результат отправки неизвестен/)
    expect(store.getState().session.drafts.writeOff.writeOff?.attempt).toEqual(attempt)
  })
  it('refuses expired-key retry and never creates a new request automatically', async () => {
    const { user } = mount({ ...initial, attempt: { key: crypto.randomUUID(), startedAt: Date.now() - 24 * 3600000, body: { batch_id: initial.batch.id, quantity: '1.125', reason: 'Порча' } } })
    await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByText(/Срок безопасного повтора истёк/); expect(requests).toHaveLength(0)
  })
  it('renders 404 safely and does not unlock a new write to a missing batch', async () => {
    setMockScenario('writeoff-not-found'); const { user } = mount(); await open(user); await fill(user); await confirm(user)
    await screen.findByText(/Партия не найдена/); expect(screen.getByRole('button', { name: 'Списать' })).toHaveProperty('disabled', true)
    expect(screen.getByLabelText(/^Причина списания/)).toHaveProperty('value', 'Порча упаковки')
  })
  it.each(['admin', 'manager', 'storekeeper'] as const)('permits eligible role %s', async (role) => {
    const { user } = mount(undefined, role); await open(user); await fill(user); await confirm(user); await screen.findByRole('dialog', { name: 'Списание выполнено' })
  })
  it('unmounts the old owner form on account change and prevents stale write or result disclosure', async () => {
    const { user, store } = mount(); await open(user); await fill(user)
    const previous = store.getState().session
    store.dispatch(replaceSession({ ...previous, user: { ...previous.user!, id: mockId(901) }, drafts: {}, draftOwner: mockId(901) }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Списать из партии' })).toBeNull())
    expect(requests).toHaveLength(0)
    await open(user); expect(screen.getByLabelText(/^Причина списания/)).toHaveProperty('value', '')
  })
  it('retains the pending attempt on session expiration and aborts the UI request', async () => {
    setMockScenario('writeoff-slow'); const { user, store } = mount(); await open(user); await fill(user); await confirm(user)
    const previous = store.getState().session
    expect(previous.drafts.writeOff.writeOff?.attempt).toBeTruthy()
    store.dispatch(replaceSession({ ...initialSessionState, status: 'anonymous', draftOwner: previous.draftOwner, drafts: previous.drafts }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Списать из партии' })).toBeNull())
    expect(store.getState().session.drafts.writeOff.writeOff?.attempt?.key).toBe(requests[0].key)
  })
  it('never mounts a write form or sends for order_operator', async () => {
    const { user } = mount(undefined, 'order_operator'); await open(user); expect(screen.queryByRole('dialog')).toBeNull(); expect(requests).toHaveLength(0)
  })
  it('refreshes every existing dependent active cache only after a validated result, including replay', async () => {
    type Tag = typeof writeOffTags[number]
    const calls = new Map<Tag, number>()
    const probeApi = baseApi.enhanceEndpoints({ addTagTypes: [...writeOffTags] }).injectEndpoints({ endpoints: (build) => ({ writeOffCacheProbe: build.query<string, Tag>({ queryFn: (tag) => { calls.set(tag, (calls.get(tag) ?? 0) + 1); return { data: tag } }, providesTags: (_result, _error, tag) => [tag] }) }) })
    const { user, store } = mount(); await open(user); await fill(user)
    const subscriptions = writeOffTags.map((tag) => store.dispatch(probeApi.endpoints.writeOffCacheProbe.initiate(tag)))
    await Promise.all(subscriptions)
    setMockScenario('writeoff-lost-response'); await confirm(user); await screen.findByText(/Результат отправки неизвестен/)
    expect([...calls.values()]).toEqual(writeOffTags.map(() => 1))
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Повторить исходный запрос' })); await screen.findByRole('dialog', { name: 'Списание выполнено' })
    await waitFor(() => expect([...calls.values()]).toEqual(writeOffTags.map(() => 2))); subscriptions.forEach((subscription) => subscription.unsubscribe())
  })
  it('blocks navigation while pending and guards unsaved values', async () => {
    const { user, router } = mount(); await open(user); await fill(user)
    await router.navigate('/elsewhere'); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' })
    await user.click(screen.getByRole('button', { name: 'Остаться' })); expect(screen.getByLabelText(/^Количество списания/)).toHaveProperty('value', '0.001125')
    setMockScenario('writeoff-slow'); await confirm(user); await router.navigate('/elsewhere')
    await screen.findByRole('dialog', { name: 'Уйти без сохранения?' })
    expect(screen.getByRole('button', { name: 'Уйти без сохранения' })).toHaveProperty('disabled', true)
    await user.click(screen.getByRole('button', { name: 'Остаться' })); await screen.findByRole('dialog', { name: 'Списание выполнено' }, { timeout: 5000 })
  })
  it.each(['writeoff-invalid'] as const)('does not trust an unrelated successful DTO (%s)', async (scenario: MockScenario) => {
    setMockScenario(scenario); const { store } = mount()
    const result = await store.dispatch(writeOffApi.endpoints.writeOff.initiate({ key: crypto.randomUUID(), body: { batch_id: initial.batch.id, quantity: '1.125', reason: 'Порча' }, productId: initial.product.id, unit: initial.batch.unit }))
    expect(result.error).toHaveProperty('code', 'INVALID_RESPONSE')
  })
})
