// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { userApi, userSchema, roleListSchema, defaultUserSearch, readUserSearch, userSearchParams, changeUserSearch, safeUserListUrl } from '@/entities/user'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { userHandlers } from '@/app/mocks/user-handlers'
import { mockUsers, mockRoles, mockUserList } from '@/app/mocks/user-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { UsersPage } from './UsersPage'
import { UserEntryPage } from './UserEntryPage'

const server = setupServer(...userHandlers), NativeRequest = globalThis.Request
const requests: Request[] = []
function makeStore(role: Role = 'admin') {
  return configureStore({ reducer: { session: sessionReducer, [userApi.reducerPath]: userApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockUsers[0].id, name: 'Тест', email: 'test@example.test', role }, settings: { timezone: 'Asia/Tokyo', restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(userApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/admin/users', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const guard = (children: React.ReactNode) => <RoleGate section={sections.find((section) => section.id === 'users')!}>{children}</RoleGate>
  const router = createMemoryRouter([
    { path: '/admin/users', element: guard(<UsersPage />) },
    { path: '/admin/users/new', element: guard(<UserEntryPage />) },
    { path: '/admin/users/:id/edit', element: guard(<UserEntryPage />) },
    { path: '/admin/users/:id/password', element: guard(<UserEntryPage password />) },
  ], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const ready = () => screen.findByRole('article', { name: `Пользователь: ${mockUsers[0].name}` })
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); requests.length = 0; document.cookie = 'flagman_mock_role=admin; Path=/' })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(userApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })

describe('F21 safe published user reads', () => {
  it('allowlists literal filters and nested returns, rejects invalid enums and numbers', () => {
    const values = readUserSearch(new URLSearchParams('search=%25_%5C&role=manager&is_active=false&sort=created_at&page=2&page_size=1&password=secret'))
    expect(values).toEqual({ search: '%_\\', role: 'manager', is_active: 'false', sort: 'name', page: 2, page_size: 1 })
    expect(changeUserSearch(values, { role: 'admin' }).page).toBe(1)
    expect(changeUserSearch(values, { page: 3 }).page).toBe(3)
    expect(readUserSearch(new URLSearchParams('role=owner&is_active=no&page=9007199254740992&page_size=101'))).toEqual(defaultUserSearch)
    const url = `/admin/users?${userSearchParams(values)}`
    expect(safeReturnUrl(`${url}&password=secret#hash`)).toBe(url)
    expect(safeReturnUrl(`/admin/users/${mockUsers[0].id}/password?${new URLSearchParams({ returnTo: url, token: 'secret' })}`)).toBe(`/admin/users/${mockUsers[0].id}/password?${new URLSearchParams({ returnTo: url })}`)
    for (const invalid of ['//outside.test', 'https://outside.test', '/orders', '/\\outside.test', '/admin/%75sers']) expect(safeUserListUrl(invalid)).toBe('/admin/users')
  })
  it('renders activity, safe email, null login and restaurant timezone rather than browser timezone', async () => {
    mount(); const anna = await ready(); await waitFor(() => expect(requests).toHaveLength(2))
    expect(anna.textContent).toContain('anna@example.test'); expect(anna.textContent).toContain('Администратор'); expect(anna.textContent).toContain('12.10.2026, 15:15:00')
    const boris = screen.getByRole('article', { name: `Пользователь: ${mockUsers[1].name}` }); expect(boris.textContent).toContain('Отключён'); expect(boris.textContent).toContain('Ещё не входил')
    expect(Object.fromEntries(new URL(requests.find((r) => new URL(r.url).pathname === '/api/v1/users')!.url).searchParams)).toEqual({ role: 'all', is_active: 'all', sort: 'name', page: '1', page_size: '50' })
    expect(requests.every((r) => r.method === 'GET' && !r.url.includes('password'))).toBe(true)
  })
  it('uses server directory labels/options and rejects invented roles', async () => {
    server.use(http.get('/api/v1/roles', () => Response.json({ items: mockRoles.items.map((role) => ({ ...role, name: `${role.name} · сервер` })) })))
    mount(); await ready(); await screen.findByRole('option', { name: 'Администратор · сервер' }); await waitFor(() => expect(screen.getByRole('article', { name: `Пользователь: ${mockUsers[0].name}` }).textContent).toContain('Администратор · сервер'))
    expect(roleListSchema.safeParse({ items: [...mockRoles.items, { code: 'owner', name: 'Владелец' }] }).success).toBe(false)
    expect(roleListSchema.safeParse({ items: [mockRoles.items[0], mockRoles.items[0], ...mockRoles.items.slice(2)] }).success).toBe(false)
  })
  it('searches both name/email literally, combines filters and preserves navigation history', async () => {
    const { user, router } = mount('/admin/users?page_size=1'); await ready()
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await screen.findByRole('article', { name: `Пользователь: ${mockUsers[1].name}` })
    await user.selectOptions(screen.getByLabelText('Роль'), 'storekeeper'); await waitFor(() => expect(router.state.location.search).not.toContain('page='))
    await user.selectOptions(screen.getByLabelText('Активность'), 'true'); await screen.findByRole('heading', { name: 'Ничего не найдено' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
    await act(async () => { await router.navigate(-1) }); await screen.findByRole('article', { name: `Пользователь: ${mockUsers[1].name}` })
    await user.click(screen.getAllByRole('button', { name: 'Сбросить фильтры' })[0]); await ready()
    await user.type(screen.getByLabelText('Поиск по имени или email'), 'VERA@EXAMPLE.TEST'); await user.keyboard('{Enter}'); await screen.findByRole('article', { name: `Пользователь: ${mockUsers[2].name}` }); expect(screen.getAllByRole('article')).toHaveLength(1)
    await user.clear(screen.getByLabelText('Поиск по имени или email')); await user.type(screen.getByLabelText('Поиск по имени или email'), '%_\\'); await user.keyboard('{Enter}'); await screen.findByRole('article', { name: 'Пользователь: Менеджер 01 %_\\' }); expect(new URLSearchParams(router.state.location.search).get('search')).toBe('%_\\')
  })
  it('has keyboard create/edit/password entries and safe return filters without writes', async () => {
    const { user, router } = mount('/admin/users?role=admin'); await ready()
    for (const [link, heading] of [[`Редактировать: ${mockUsers[0].name}`, 'Редактирование пользователя'], [`Сбросить пароль: ${mockUsers[0].name}`, 'Сброс пароля пользователя'], ['Создать пользователя', 'Создание пользователя']]) {
      screen.getByRole('link', { name: link }).focus(); await user.keyboard('{Enter}'); await screen.findByRole('heading', { name: heading })
      expect(screen.getByRole('heading', { name: 'Форма будет доступна в следующей истории' })).toBeTruthy()
      await user.click(screen.getByRole('link', { name: 'В список пользователей' })); await ready(); expect(router.state.location.search).toBe('?role=admin')
    }
    expect(requests.every((r) => r.method === 'GET' && !/\/users\//.test(r.url))).toBe(true)
  })
  it('clears unsubmitted search on reset even when the URL already has default filters', async () => {
    const { user, router } = mount(); await ready()
    const search = screen.getByLabelText('Поиск по имени или email') as HTMLInputElement
    await user.type(search, 'pending@example.test')
    await user.click(screen.getAllByRole('button', { name: 'Сбросить фильтры' })[0])
    expect(search.value).toBe(''); expect(router.state.location.search).toBe('')
    expect(screen.getAllByRole('article')).toHaveLength(25)
  })
  it('preserves same-filter stale records, retries and isolates new failed parameters', async () => {
    const { user } = mount(); await ready(); setMockScenario('users-error')
    await user.click(screen.getByRole('button', { name: 'Обновить пользователей' })); await screen.findByText(/Данные могут быть устаревшими/); expect(screen.getAllByRole('article')).toHaveLength(25)
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await waitFor(() => expect(screen.queryByText(/Данные могут быть устаревшими/)).toBeNull())
    setMockScenario('users-error'); await user.selectOptions(screen.getByLabelText('Активность'), 'false'); await screen.findByRole('heading', { name: 'Не удалось загрузить пользователей' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('can read users while the directory fails, disables role editing and retries independently', async () => {
    setMockScenario('roles-error'); const { user } = mount('/admin/users?role=admin'); await ready(); await screen.findByRole('heading', { name: 'Не удалось загрузить роли' }); expect((screen.getByLabelText('Роль') as HTMLSelectElement).disabled).toBe(true)
    expect(screen.getByRole('article').textContent).toContain('Администратор'); setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await waitFor(() => expect((screen.getByLabelText('Роль') as HTMLSelectElement).disabled).toBe(false))
    expect(requests.filter((r) => new URL(r.url).pathname === '/api/v1/users')).toHaveLength(1)
  })
  it.each(['forbidden', 'roles-forbidden'] as const)('hides previously cached personal records on %s', async (scenario) => {
    const { store } = mount(); await ready(); setMockScenario(scenario); act(() => { store.dispatch(userApi.util.invalidateTags(['Users', 'Roles'])) })
    await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryAllByRole('article')).toHaveLength(0); expect(screen.queryByText('anna@example.test')).toBeNull()
  })
  it('recovers an empty later page and distinguishes the empty catalogue', async () => {
    const { user } = mount('/admin/users?page=99'); await screen.findByRole('heading', { name: 'На этой странице нет пользователей' }); await user.click(screen.getByRole('button', { name: 'К первой странице' })); await ready()
    setMockScenario('users-empty'); await user.click(screen.getByRole('button', { name: 'Обновить пользователей' })); await screen.findByRole('heading', { name: 'Список пользователей пуст' })
  })
  it.each(['manager', 'storekeeper', 'order_operator'] as const)('denies %s before list/directory/entry requests', async (role) => {
    const { router } = mount('/admin/users', role); await screen.findByRole('heading', { name: 'Недостаточно прав' })
    for (const path of ['/admin/users/new', `/admin/users/${mockUsers[0].id}/edit`, `/admin/users/${mockUsers[0].id}/password`]) { await act(async () => { await router.navigate(path) }); expect(screen.getByRole('heading', { name: 'Недостаточно прав' })).toBeTruthy() }
    expect(requests).toHaveLength(0)
  })
  it.each(['password', 'password_hash', 'csrf_token'] as const)('rejects response %s before query cache or UI receives it', async (field) => {
    const unsafe = { ...mockUsers[0], [field]: 'must-never-be-cached' }; expect(userSchema.safeParse(unsafe).success).toBe(false)
    server.use(http.get('/api/v1/users', () => Response.json({ items: [unsafe], total: 1, page: 1, page_size: 50, meta: {} })))
    const { store } = mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить пользователей' }); expect(screen.queryAllByRole('article')).toHaveLength(0); expect(JSON.stringify(store.getState())).not.toContain('must-never-be-cached')
  })
  it('rejects invalid dates/roles and mismatched pagination, duplicate UUIDs and filter rows', async () => {
    expect(userSchema.safeParse({ ...mockUsers[0], last_login_at: '2026-02-30T00:00:00Z' }).success).toBe(false)
    expect(userSchema.safeParse({ ...mockUsers[0], role: 'owner' }).success).toBe(false)
    const store = makeStore(); stores.push(store)
    for (const body of [{ ...mockUserList(defaultUserSearch), page: 2 }, { items: [mockUsers[0], { ...mockUsers[0], id: mockUsers[0].id.toUpperCase() }], total: 2, page: 1, page_size: 50, meta: {} }, { items: [mockUsers[1]], total: 1, page: 1, page_size: 50, meta: {} }]) {
      server.use(http.get('/api/v1/users', () => Response.json(body)))
      const request = store.dispatch(userApi.endpoints.users.initiate({ ...defaultUserSearch, role: body.items[0].role === 'storekeeper' ? 'admin' : 'all' }, { forceRefetch: true }))
      const result = await request; request.unsubscribe(); expect(result.error).toMatchObject({ status: 'INVALID_RESPONSE' })
    }
  })
  it('refreshes active users after invalidation without another role read', async () => {
    const { store } = mount(); await ready(); await waitFor(() => expect(requests).toHaveLength(2)); setMockScenario('users-empty')
    act(() => { store.dispatch(userApi.util.invalidateTags(['Users'])) }); await screen.findByRole('heading', { name: 'Список пользователей пуст' }); expect(requests.filter((r) => new URL(r.url).pathname === '/api/v1/roles')).toHaveLength(1)
  })
  it('keeps entries honest and validates UUID without detail reads', async () => {
    mount('/admin/users/not-a-uuid/password'); await screen.findByRole('heading', { name: 'Некорректный адрес пользователя' }); expect(screen.queryByRole('textbox')).toBeNull()
    expect(requests).toHaveLength(0)
  })
})
