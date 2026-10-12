// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { baseApi } from '@/shared/api'
import { initialSessionState, sessionReducer } from '@/entities/session'
import { settingsApi } from '@/entities/restaurant-settings'
import { dashboardApi } from '@/entities/dashboard'
import { settingsHandlers } from '@/app/mocks/settings-handlers'
import { mockSettings, resetSettingsFixtures, saveMockSettings } from '@/app/mocks/settings-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { SettingsPage } from '@/pages/admin-settings'
import { SettingsEditor } from './SettingsEditor'

const server = setupServer(...settingsHandlers), NativeRequest = globalThis.Request
const requests: unknown[] = [], stores: { dispatch: (action: ReturnType<typeof baseApi.util.resetApiState>) => unknown }[] = []
function mount(ready?: boolean) {
  const store = configureStore({ reducer: { session: sessionReducer, [baseApi.reducerPath]: dashboardApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'local@example.test', role: 'admin' as const } } }, middleware: (getDefault) => getDefault({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests' } } }).concat(dashboardApi.middleware) })
  const router = createMemoryRouter([{ path: '/admin/settings', element: ready === undefined ? <SettingsPage /> : <SettingsEditor initial={mockSettings()} autoWriteoffReady={ready} onReload={async () => { const result = await store.dispatch(settingsApi.endpoints.restaurantSettings.initiate(undefined, { forceRefetch: true, subscribe: false })); return result.error ? undefined : result.data }} /> }, { path: '/', element: <h1>Обзор</h1> }], { initialEntries: ['/admin/settings'] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>); stores.push(store)
  return { store, router, user: userEvent.setup() }
}
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' })
  server.events.on('request:start', async ({ request }) => { if (request.method === 'PATCH') requests.push(await request.clone().json()) })
})
beforeEach(() => { resetSettingsFixtures(); setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(baseApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
const save = () => screen.getByRole('button', { name: 'Сохранить настройки' })
async function rename(user: ReturnType<typeof userEvent.setup>) { const input = await screen.findByLabelText(/^Название ресторана/); await user.clear(input); await user.type(input, 'Новая кухня') }
describe('settings UI with explicit MSW', () => {
  it('validates keyboard submits, saves only changed fields and updates safe session', async () => {
    const { user, store } = mount(); await rename(user)
    const threshold = screen.getByLabelText(/^Порог предупреждения/); await user.clear(threshold); await user.type(threshold, '721'); await user.click(save())
    expect(await screen.findByText('Порог: от 1 до 720 часов.')).toBeTruthy(); expect(requests).toHaveLength(0)
    await user.clear(threshold); await user.type(threshold, '720'); await user.click(save())
    await screen.findByText(/Настройки сохранены/); expect(requests).toEqual([{ restaurant_name: 'Новая кухня', expiring_threshold_hours: 720, expected_revision: 1 }])
    expect(store.getState().session.settings).toMatchObject({ restaurantName: 'Новая кухня', expiringThresholdHours: 720, revision: 2 }); expect(save()).toHaveProperty('disabled', true)
  })
  it('does not enable an unconfirmed worker', async () => {
    const { user } = mount(false); expect(screen.getByLabelText('Автоматически списывать просроченные партии')).toHaveProperty('disabled', true)
    await rename(user); await user.click(save()); await screen.findByText(/Настройки сохранены/); expect(requests[0]).toEqual({ restaurant_name: 'Новая кухня', expected_revision: 1 })
  })
  it('can disable an already enabled function when readiness is unknown', async () => {
    saveMockSettings({ expected_revision: 1, auto_writeoff_expired: true })
    const { user } = mount(false); const auto = screen.getByLabelText('Автоматически списывать просроченные партии')
    expect(auto).toHaveProperty('disabled', false); await user.click(auto); await user.click(save()); await screen.findByText(/Настройки сохранены/)
    expect(requests[0]).toEqual({ expected_revision: 2, auto_writeoff_expired: false })
  })
  it('requires confirmation for ready worker and preserves fields on FEATURE_UNAVAILABLE', async () => {
    const { user } = mount(true); await rename(user); await user.click(screen.getByLabelText('Автоматически списывать просроченные партии')); setMockScenario('settings-feature-unavailable'); await user.click(save()); expect(requests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Включить и сохранить' })); await screen.findByText(/Фоновое списание пока недоступно/); expect(screen.getByLabelText(/^Название ресторана/)).toHaveProperty('value', 'Новая кухня'); expect(save()).toHaveProperty('disabled', true)
    await user.click(screen.getByLabelText('Автоматически списывать просроченные партии')); await user.click(save()); await screen.findByText(/Настройки сохранены/); expect(mockSettings().auto_writeoff_expired).toBe(false)
  })
  it('retains values across conflict and failed reload, requires explicit revision review', async () => {
    const { user } = mount(); await rename(user); setMockScenario('settings-revision-conflict'); await user.click(save()); await screen.findByText(/Настройки изменены другим/)
    expect(save()).toHaveProperty('disabled', true); setMockScenario('settings-read-error'); await user.click(screen.getByRole('button', { name: 'Прочитать актуальные настройки' })); await screen.findByText(/Не удалось прочитать актуальные/)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Прочитать актуальные настройки' })); await screen.findByRole('region', { name: 'Актуальные настройки' }); expect(screen.getByLabelText(/^Название ресторана/)).toHaveProperty('value', 'Новая кухня'); expect(save()).toHaveProperty('disabled', true)
    await user.click(screen.getByRole('button', { name: 'Продолжить с этой ревизией' })); await user.click(save()); await screen.findByText(/Настройки сохранены/); expect(requests[1]).toEqual({ restaurant_name: 'Новая кухня', expected_revision: 2 })
  })
  it('maps only allowlisted 422 paths and preserves input', async () => {
    const { user } = mount(); await rename(user); setMockScenario('settings-validation'); await user.click(save()); await screen.findByText('Проверьте поля формы. Изменения не сохранены.'); expect(document.activeElement).toBe(screen.getByLabelText(/^Порог предупреждения/)); expect(screen.getByLabelText(/^Название ресторана/)).toHaveProperty('value', 'Новая кухня')
  })
  it('locks unknown outcomes without retry and reviews the saved server revision', async () => {
    const { user } = mount(); await rename(user); setMockScenario('settings-lost-response'); await user.click(save()); await screen.findByText(/Результат сохранения неизвестен/); expect(save()).toHaveProperty('disabled', true); expect(requests).toHaveLength(1)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Прочитать актуальные настройки' })); await screen.findByRole('heading', { name: 'Проверьте ревизию 2' }); expect(requests).toHaveLength(1)
  })
  it('deduplicates pending writes and protects dirty navigation', async () => {
    const { user, router } = mount(); await rename(user); void router.navigate('/'); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' }); await user.click(screen.getByRole('button', { name: 'Остаться' })); setMockScenario('settings-save-slow'); await user.dblClick(save()); expect(screen.getByRole('button', { name: 'Сохраняем настройки…' })).toHaveProperty('disabled', true); await screen.findByText(/Настройки сохранены/, {}, { timeout: 3500 }); expect(requests).toHaveLength(1)
  })
  it('retries failed initial read', async () => {
    setMockScenario('settings-read-error'); const { user } = mount(); await screen.findByRole('heading', { name: 'Не удалось прочитать настройки' }); expect(screen.queryByLabelText(/^Название ресторана/)).toBeNull(); setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByLabelText(/^Название ресторана/)
  })
  it('rejects malformed settings instead of mounting a partial form', async () => {
    setMockScenario('invalid-schema'); mount(); await screen.findByRole('heading', { name: 'Не удалось прочитать настройки' }); expect(screen.queryByLabelText(/^Название ресторана/)).toBeNull()
  })
  it('refreshes an active server snapshot after save and after checking a lost response', async () => {
    let reads = 0
    server.use(http.get('/api/v1/dashboard', () => {
      reads++
      const { restaurant_name, timezone, expiring_threshold_hours } = mockSettings()
      return Response.json({ expiring_batches_count: 0, expired_batches_count: 0, stop_list_dishes_count: 0, out_of_stock_products_count: 0, attention_batches: [], attention_total: 0, hot_offers: [], procurement_recommendations: [], settings: { restaurant_name, timezone, expiring_threshold_hours }, meta: { server_time: '2099-01-01T00:00:00Z' } })
    }))
    const { user, store } = mount(); await rename(user)
    const subscription = store.dispatch(dashboardApi.endpoints.dashboard.initiate())
    await subscription.unwrap(); expect(reads).toBe(1)
    await user.click(save()); await screen.findByText(/Настройки сохранены/)
    await waitFor(() => expect(dashboardApi.endpoints.dashboard.select()(store.getState()).data?.settings.restaurant_name).toBe('Новая кухня')); expect(reads).toBe(2)
    const name = screen.getByLabelText(/^Название ресторана/); await user.clear(name); await user.type(name, 'Потерянный ответ'); setMockScenario('settings-lost-response'); await user.click(save()); await screen.findByText(/Результат сохранения неизвестен/); expect(reads).toBe(2)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Прочитать актуальные настройки' }))
    await waitFor(() => expect(dashboardApi.endpoints.dashboard.select()(store.getState()).data?.settings.restaurant_name).toBe('Потерянный ответ')); expect(reads).toBe(3); subscription.unsubscribe()
  })
  it('rejects a success response with mismatched fields/revision', async () => {
    const { user } = mount(); await rename(user); server.use(http.patch('/api/v1/settings', () => Response.json(mockSettings()))); await user.click(save()); await screen.findByText(/Результат сохранения неизвестен/); expect(save()).toHaveProperty('disabled', true)
  })
  it('does not refresh a detached revision on focus or sibling cache invalidation', async () => {
    const { user, store } = mount(); await rename(user); store.dispatch(settingsApi.util.invalidateTags(['Settings', 'Dashboard'])); window.dispatchEvent(new Event('focus')); await waitFor(() => expect(screen.getByLabelText(/^Название ресторана/)).toHaveProperty('value', 'Новая кухня')); await user.click(save()); await screen.findByText(/Настройки сохранены/); expect(requests[0]).toEqual({ restaurant_name: 'Новая кухня', expected_revision: 1 })
  })
})
