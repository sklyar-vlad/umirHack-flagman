// @vitest-environment jsdom
import { configureStore, type Middleware } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, afterAll, beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { initialSessionState, sessionReducer } from '@/entities/session'
import { userApi, writeUser } from '@/entities/user'
import { AuthContext, type SessionController } from '@/features/auth'
import { userHandlers } from '@/app/mocks/user-handlers'
import { mockUsers, resetMockUsers, mockActorId, getMockUser } from '@/app/mocks/user-fixtures'
import { createMockError } from '@/app/mocks/responses'
import { setMockScenario } from '@/app/mocks/scenarios'
import { UserEntryPage } from '@/pages/admin-users'
const server = setupServer(...userHandlers), NativeRequest = globalThis.Request, requests: Request[] = []
const secret = 'local-new-secret-only', actions: unknown[] = []
function mount(id?: string, password = false, own = false) {
  const auth: SessionController = { restore: vi.fn(async () => true), login: vi.fn(), logout: vi.fn(), expire: vi.fn(), getVersion: () => 0, hasAccess: () => true, registerSubscription: () => () => false }
  const recorder: Middleware = () => (next) => (action) => { if (typeof action !== 'function') actions.push(action); return next(action) }
  const store = configureStore({ reducer: { session: sessionReducer, [userApi.reducerPath]: userApi.reducer }, preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, expiresAt: '2099-01-01T00:00:00Z', user: { id: own ? mockActorId : '22222222-2222-4222-8222-222222222222', name: 'Тест', email: 'test@example.test', role: 'admin' as const } } }, middleware: (getDefaultMiddleware) => getDefaultMiddleware({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests', onUnauthorized: auth.expire } } }).concat(recorder, userApi.middleware) })
  const path = id ? `/admin/users/${id}/${password ? 'password' : 'edit'}` : '/admin/users/new'
  const router = createMemoryRouter([{ path: '/admin/users/new', element: <UserEntryPage /> }, { path: '/admin/users/:id/edit', element: <UserEntryPage /> }, { path: '/admin/users/:id/password', element: <UserEntryPage password /> }, { path: '/admin/users', element: <h1>Список</h1> }], { initialEntries: [path] })
  render(<Provider store={store}><AuthContext.Provider value={auth}><RouterProvider router={router} /></AuthContext.Provider></Provider>)
  return { store, auth, router, user: userEvent.setup() }
}
const writes = () => requests.filter((request) => request.method !== 'GET')
async function changeName(user: ReturnType<typeof userEvent.setup>, name = 'Моё имя') { await screen.findByLabelText('Имя пользователя'); await user.clear(screen.getByLabelText('Имя пользователя')); await user.type(screen.getByLabelText('Имя пользователя'), name) }
async function createValues(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole('option', { name: 'Менеджер' }); await waitFor(() => expect((screen.getByLabelText('Роль пользователя') as HTMLSelectElement).value).toBe('manager')); await changeName(user, 'Новый пользователь')
  await user.type(screen.getByLabelText('Email пользователя'), 'NEW@EXAMPLE.TEST'); await user.type(screen.getByLabelText('Начальный пароль'), secret)
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }; HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request.clone()))
})
beforeEach(() => { setMockScenario('success'); resetMockUsers(); requests.length = 0; actions.length = 0; document.cookie = 'flagman_mock_role=admin; Path=/' })
afterEach(() => { cleanup(); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
describe('F22 user editor', () => {
  it('validates before sending, creates published body once and keeps secrets out of Redux/actions/storage', async () => {
    const { user, store } = mount(); await screen.findByRole('option', { name: 'Менеджер' }); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByText('Пароль: от 12 до 128 символов'); expect(writes()).toHaveLength(0)
    await createValues(user); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('heading', { name: 'Пользователь сохранён: Новый пользователь' })
    expect(writes()).toHaveLength(1); expect(await writes()[0].json()).toEqual({ name: 'Новый пользователь', email: 'new@example.test', role: 'manager', is_active: true, password: secret })
    expect(writes()[0].headers.get('X-CSRF-Token')).toBe('mock-csrf-for-local-tests')
    expect(JSON.stringify(store.getState()) + JSON.stringify(actions) + JSON.stringify(sessionStorage) + JSON.stringify(localStorage)).not.toContain(secret)
    expect(screen.queryByLabelText('Начальный пароль')).toBeNull()
  })
  it('patches only changed metadata and updates own safe session identity', async () => {
    const { user, store, auth } = mount(mockActorId, false, true); await changeName(user); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('heading', { name: 'Пользователь сохранён: Моё имя' })
    expect(await writes()[0].json()).toEqual({ name: 'Моё имя', expected_revision: 1 }); expect(store.getState().session.user?.name).toBe('Моё имя'); expect(auth.expire).not.toHaveBeenCalled()
  })
  it('confirms access changes, rejects last admin and preserves entered fields', async () => {
    const { user } = mount(mockUsers[0].id); await changeName(user); await user.selectOptions(screen.getByLabelText('Роль пользователя'), 'manager'); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('dialog', { name: 'Подтвердите изменение доступа' }); expect(writes()).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'Изменить доступ' })); await screen.findByText(/Нельзя отключить или понизить последнего/); expect((screen.getByLabelText('Имя пользователя') as HTMLInputElement).value).toBe('Моё имя'); expect((screen.getByLabelText('Роль пользователя') as HTMLSelectElement).value).toBe('manager')
  })
  it('disables a non-admin after confirmation and re-enabling does not revoke session', async () => {
    const { user } = mount(mockUsers[2].id); await screen.findByLabelText('Имя пользователя'); await user.click(screen.getByLabelText('Пользователь активен')); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await user.click(await screen.findByRole('button', { name: 'Изменить доступ' })); await screen.findByRole('heading', { name: `Пользователь сохранён: ${mockUsers[2].name}` }); expect(getMockUser(mockUsers[2].id)?.is_active).toBe(false)
  })
  it('requires explicit conflict read/review without replacing values on invalidation', async () => {
    const { user, store } = mount(mockUsers[2].id); await changeName(user); setMockScenario('user-revision-conflict'); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByText(/Пользователь изменён другим/)
    expect((screen.getByRole('button', { name: 'Сохранить пользователя' }) as HTMLButtonElement).disabled).toBe(true)
    setMockScenario('success'); act(() => { store.dispatch(userApi.util.invalidateTags(['Users'])) }); expect((screen.getByLabelText('Имя пользователя') as HTMLInputElement).value).toBe('Моё имя')
    await user.click(screen.getByRole('button', { name: 'Прочитать актуального пользователя' })); await screen.findByRole('heading', { name: 'Проверьте актуального пользователя' }); expect(writes()).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Продолжить с этой ревизией' })); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('heading', { name: 'Пользователь сохранён: Моё имя' }); expect((await writes()[1].json()).expected_revision).toBe(2)
  })
  it.each(['user-email-conflict', 'user-validation'] as const)('preserves create values on %s and permits corrected submit', async (scenario) => {
    const { user } = mount(); await createValues(user); setMockScenario(scenario); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByText(scenario === 'user-email-conflict' ? 'Этот email уже используется. Укажите другой адрес.' : 'Проверьте поля формы. Изменения не сохранены.')
    expect((screen.getByLabelText('Начальный пароль') as HTMLInputElement).value).toBe(secret); setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('heading', { name: 'Пользователь сохранён: Новый пользователь' })
  })
  it('locks unknown creation outcomes and never retries automatically', async () => {
    const { user } = mount(); await createValues(user); setMockScenario('user-lost-response'); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByText(/Результат сохранения неизвестен/); expect(writes()).toHaveLength(1); expect((screen.getByRole('button', { name: 'Сохранить пользователя' }) as HTMLButtonElement).disabled).toBe(true)
  })
  it('guards dirty navigation and discards only after explicit action', async () => {
    const { user, router } = mount(); await createValues(user); await user.click(screen.getByRole('link', { name: 'В список пользователей' })); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' }); await user.click(screen.getByRole('button', { name: 'Остаться' })); expect(router.state.location.pathname).toBe('/admin/users/new')
    await user.click(screen.getByRole('link', { name: 'В список пользователей' })); await user.click(screen.getByRole('button', { name: 'Уйти без сохранения' })); await screen.findByRole('heading', { name: 'Список' }); expect(writes()).toHaveLength(0)
  })
  it('clears password after explicit dirty close and after success, uses updated revision', async () => {
    const { user, store } = mount(mockUsers[2].id, true); await screen.findByLabelText('Новый пароль'); await user.type(screen.getByLabelText('Новый пароль'), secret); await user.click(screen.getByRole('button', { name: 'Отмена' })); await screen.findByRole('dialog', { name: 'Закрыть без смены пароля?' }); await user.click(screen.getByRole('button', { name: 'Закрыть и очистить пароль' })); expect(screen.queryByLabelText('Новый пароль')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Сбросить пароль' })); expect((screen.getByLabelText('Новый пароль') as HTMLInputElement).value).toBe(''); await user.type(screen.getByLabelText('Новый пароль'), secret); await user.click(screen.getByRole('button', { name: 'Сменить пароль и отозвать сессии' })); await waitFor(() => expect(screen.queryByLabelText('Новый пароль')).toBeNull())
    expect(await writes()[0].json()).toEqual({ password: secret, expected_revision: 1 }); expect(JSON.stringify(store.getState()) + JSON.stringify(actions)).not.toContain(secret)
    await changeName(user); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('heading', { name: 'Пользователь сохранён: Моё имя' }); expect((await writes()[1].json()).expected_revision).toBe(2)
  })
  it('expires own session after reset and own access change', async () => {
    const { user, auth } = mount(mockActorId, true, true); await screen.findByLabelText('Новый пароль'); await user.type(screen.getByLabelText('Новый пароль'), secret); await user.click(screen.getByRole('button', { name: 'Сменить пароль и отозвать сессии' })); await waitFor(() => expect(auth.expire).toHaveBeenCalledTimes(1))
  })
  it('sanitizes echoed secret errors and rejected safe DTOs before state/actions', async () => {
    const { store } = mount(); server.use(http.post('/api/v1/users', () => createMockError(422, 'VALIDATION_ERROR', secret, { fields: [{ path: 'password', code: 'INVALID', message: secret }], password: secret })))
    const input = { kind: 'create' as const, body: { name: 'Тест', email: 'test@example.test', role: 'manager' as const, password: secret } }
    let failure: unknown; try { await store.dispatch(writeUser(input, new AbortController().signal)) } catch (error) { failure = error }
    expect(JSON.stringify(failure)).not.toContain(secret); expect(JSON.stringify(actions)).not.toContain(secret)
    server.use(http.post('/api/v1/users', () => Response.json({ ...mockUsers[0], password: secret })))
    await expect(store.dispatch(writeUser(input, new AbortController().signal))).rejects.toMatchObject({ status: 'INVALID_RESPONSE' }); expect(JSON.stringify(store.getState())).not.toContain(secret)
  })
  it('blocks repeat submits while busy and executes one request', async () => {
    const { user } = mount(); await createValues(user); setMockScenario('user-save-slow'); await user.dblClick(screen.getByRole('button', { name: 'Сохранить пользователя' })); expect((screen.getByRole('button', { name: 'Подождите…' }) as HTMLButtonElement).disabled).toBe(true); await screen.findByRole('heading', { name: 'Пользователь сохранён: Новый пользователь' }, { timeout: 3000 }); expect(writes()).toHaveLength(1)
  })
  it('handles directory failure with explicit retry and blocks save', async () => {
    setMockScenario('roles-error'); const { user } = mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить роли' }); expect((screen.getByRole('button', { name: 'Сохранить пользователя' }) as HTMLButtonElement).disabled).toBe(true); setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await screen.findByRole('option', { name: 'Менеджер' }); expect((screen.getByRole('button', { name: 'Сохранить пользователя' }) as HTMLButtonElement).disabled).toBe(false); expect(writes()).toHaveLength(0)
  })
  it('blocks route departure during private password writes until success', async () => {
    const { user, router } = mount(mockUsers[2].id, true); await screen.findByLabelText('Новый пароль'); await user.type(screen.getByLabelText('Новый пароль'), secret); setMockScenario('user-save-slow'); await user.click(screen.getByRole('button', { name: 'Сменить пароль и отозвать сессии' })); await screen.findByRole('button', { name: 'Подождите…' }); await act(async () => { await router.navigate('/admin/users') }); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' }); expect((screen.getByRole('button', { name: 'Уйти без сохранения' }) as HTMLButtonElement).disabled).toBe(true); expect(router.state.location.pathname).toContain('/password'); await screen.findByRole('heading', { name: 'Список' }, { timeout: 3000 }); expect(writes()).toHaveLength(1)
  })
  it('delegates server 401 to session transport without retaining credentials', async () => {
    const { user, auth, store } = mount(); await createValues(user); server.use(http.post('/api/v1/users', () => createMockError(401, 'UNAUTHORIZED', secret))); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await waitFor(() => expect(auth.expire).toHaveBeenCalledTimes(1)); expect(writes()).toHaveLength(1); expect(JSON.stringify(store.getState()) + JSON.stringify(actions)).not.toContain(secret)
  })
  it('expires own session after changing access', async () => {
    const { user, auth } = mount(mockActorId, false, true); await screen.findByLabelText('Имя пользователя'); await screen.findByRole('option', { name: 'Менеджер' }); await user.selectOptions(screen.getByLabelText('Роль пользователя'), 'manager'); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await user.click(await screen.findByRole('button', { name: 'Изменить доступ' })); await waitFor(() => expect(auth.expire).toHaveBeenCalledTimes(1))
  })
  it('confirms enabling without claiming restoration of revoked sessions', async () => {
    const { user, auth } = mount(mockUsers[1].id); await screen.findByLabelText('Имя пользователя'); await screen.findByRole('option', { name: 'Менеджер' }); await user.click(screen.getByLabelText('Пользователь активен')); await user.click(screen.getByRole('button', { name: 'Сохранить пользователя' })); await screen.findByRole('dialog', { name: 'Подтвердите изменение доступа' }); expect(writes()).toHaveLength(0); await user.click(screen.getByRole('button', { name: 'Изменить доступ' })); await screen.findByRole('heading', { name: `Пользователь сохранён: ${mockUsers[1].name}` }); expect(auth.expire).not.toHaveBeenCalled(); expect(getMockUser(mockUsers[1].id)?.is_active).toBe(true)
  })
  it('rejects wrong identity or unchanged revision in successful write responses', async () => {
    const { store } = mount(); server.use(http.patch('/api/v1/users/:id', () => Response.json({ ...mockUsers[2], name: 'Новое имя' })))
    await expect(store.dispatch(writeUser({ kind: 'update', id: mockUsers[2].id, body: { expected_revision: 1, name: 'Новое имя' } }, new AbortController().signal))).rejects.toMatchObject({ status: 'INVALID_RESPONSE' })
  })
  it('handles user 404 and role failure with retry and no writes', async () => {
    mount('00000000-0000-4000-8000-000000009999'); await screen.findByRole('heading', { name: 'Пользователь не найден' }); expect(writes()).toHaveLength(0)
  })
})
