// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { dishApi, defaultDishSearch, readDishSearch, dishSearchParams, changeDishSearch, safeDishListUrl } from '@/entities/dish'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { dishCatalog } from '@/app/mocks/dish-catalog-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { DishesPage } from './DishesPage'
import { DishEntryPage } from './DishEntryPage'
const server = setupServer(...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: Request[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [dishApi.reducerPath]: dishApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: compositionDishes()[0].id, name: 'Тест', email: 'test@example.test', role } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(dishApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/admin/dishes', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const guard = (children: React.ReactNode) => <RoleGate section={sections.find((section) => section.id === 'recipes')!}>{children}</RoleGate>
  const router = createMemoryRouter([
    { path: '/admin/dishes', element: guard(<DishesPage />) },
    { path: '/admin/dishes/new', element: guard(<DishEntryPage />) },
    { path: '/admin/dishes/:id/edit', element: guard(<DishEntryPage />) },
  ], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const card = (name = 'Суп дня') => screen.findByRole('article', { name: `Блюдо: ${name}` })
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  HTMLDialogElement.prototype.showModal = function () { this.open = true }
  HTMLDialogElement.prototype.close = function () { this.open = false }
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dishApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
describe('F19 published administrative catalogue', () => {
  it('allowlists URL, defaults, safe return and reset-on-filter pagination', () => {
    const values = readDishSearch(new URLSearchParams('search=%25_&is_active=false&availability=unavailable&sort=portions&page=2&page_size=1&password=secret'))
    expect(values).toEqual({ search: '%_', is_active: 'false', availability: 'unavailable', sort: 'name', page: 2, page_size: 1 })
    expect(changeDishSearch(values, { availability: 'all' }).page).toBe(1)
    expect(readDishSearch(new URLSearchParams('is_active=no&availability=no&page=9007199254740992&page_size=101'))).toEqual(defaultDishSearch)
    const url = `/admin/dishes?${dishSearchParams(values)}`
    expect(safeReturnUrl(`${url}&password=secret`)).toBe(url)
    expect(safeReturnUrl(`/admin/dishes/new?${new URLSearchParams({ returnTo: url, token: 'secret' })}`)).toBe(`/admin/dishes/new?${new URLSearchParams({ returnTo: url })}`)
    expect(safeDishListUrl('//outside.test')).toBe('/admin/dishes')
    expect(safeDishListUrl('/orders')).toBe('/admin/dishes')
    expect(safeReturnUrl('/admin/recipes?is_active=false&token=secret')).toBe('/admin/dishes?is_active=false')
  })
  it('shows published descriptions, inactive and recipe_missing without order requests', async () => {
    mount(); const soup = await card()
    expect(soup.textContent).toContain('Горячий суп'); expect(soup.textContent).toContain('Ингредиентов1')
    expect((await card('Отключённое блюдо')).textContent).toContain('Отключено')
    const missing = await card('Без рецептуры'); expect(missing.textContent).toContain('Рецептура отсутствует'); expect(missing.textContent).toContain('Доступные порции0')
    expect(screen.queryByRole('button', { name: /Удалить|Добавить в заказ/ })).toBeNull()
    expect(requests).toHaveLength(1)
    expect(Object.fromEntries(new URL(requests[0].url).searchParams)).toEqual({ availability: 'all', is_active: 'all', sort: 'name', page: '1', page_size: '50' })
  })
  it('preserves large exact portions and server ingredient count', async () => {
    const dish = { ...dishCatalog(true)[0], available_portions: '900719925474099312345' }
    server.use(http.get('/api/v1/dishes', () => Response.json({ items: [dish], total: 1, page: 1, page_size: 50, meta: dish.meta })))
    mount(); const result = await card(); expect(result.textContent).toContain('900719925474099312345'); expect(result.textContent).toContain('Ингредиентов2')
  })
  it('supports pagination, combined filters, literal search and history', async () => {
    const { user, router } = mount('/admin/dishes?page_size=1'); await card('Без рецептуры')
    await user.click(screen.getByRole('button', { name: 'Страница 2' })); await card('Отключённое блюдо')
    await user.selectOptions(screen.getByLabelText('Активность'), 'false'); await card('Отключённое блюдо'); expect(router.state.location.search).not.toContain('page=')
    await user.selectOptions(screen.getByLabelText('Доступность'), 'available'); await screen.findByRole('heading', { name: 'Ничего не найдено' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
    await act(async () => { await router.navigate(-1) }); await card('Отключённое блюдо')
    await user.type(screen.getByLabelText('Поиск блюда'), '%_'); await user.click(screen.getByRole('button', { name: 'Найти' })); await screen.findByRole('heading', { name: 'Ничего не найдено' })
  })
  it('routes to editor entries and returns to the same filters without writes', async () => {
    const { user, router } = mount('/admin/dishes?search=Суп'); await card()
    await user.click(screen.getByRole('link', { name: 'Редактировать: Суп дня' })); await screen.findByRole('heading', { name: 'Редактирование блюда' })
    expect(router.state.location.pathname).toMatch(/\/admin\/dishes\/.*\/edit$/)
    await user.click(screen.getByRole('link', { name: 'В каталог блюд' })); await card(); expect(new URLSearchParams(router.state.location.search).get('search')).toBe('Суп')
    await user.click(screen.getByRole('link', { name: 'Создать блюдо' })); await screen.findByRole('heading', { name: 'Создание блюда' }); expect(requests.every((request) => request.method === 'GET')).toBe(true)
  })
  it('loads fresh preview and returns focus, retaining each exact quantity', async () => {
    const { user } = mount(); await card(); setMockScenario('dish-catalog-new-version')
    const opener = screen.getByRole('button', { name: 'Состав: Суп дня' }); await user.click(opener)
    const dialog = await screen.findByRole('dialog'); await within(dialog).findByText('Сахар — 0,001 г на порцию')
    expect(dialog.textContent).toContain('Версия рецептуры: 2'); expect(requests.filter((request) => new URL(request.url).pathname.endsWith('/800'))).toHaveLength(0)
    expect(requests.filter((request) => /\/dishes\//.test(request.url))).toHaveLength(1)
    await user.click(within(dialog).getByRole('button', { name: 'Закрыть' })); expect(document.activeElement).toBe(opener)
  })
  it.each(['dish-detail-error', 'dish-detail-missing', 'forbidden'] as const)('hides cached preview on %s', async (scenario) => {
    const { user } = mount(); await card(); await user.click(screen.getByRole('button', { name: 'Состав: Суп дня' })); await screen.findByText(/Молоко.*на порцию/)
    await user.click(screen.getByRole('button', { name: 'Закрыть' })); setMockScenario(scenario)
    await user.click(screen.getByRole('button', { name: 'Состав: Суп дня' }))
    await screen.findByRole('heading', { name: scenario === 'dish-detail-error' ? 'Не удалось загрузить состав' : scenario === 'dish-detail-missing' ? 'Блюдо не найдено' : 'Недостаточно прав' })
    expect(screen.queryByText(/Молоко.*на порцию/)).toBeNull()
  })
  it('retains same-filter stale data, retries, and isolates new parameters', async () => {
    const { user } = mount(); await card(); setMockScenario('dishes-error')
    await user.click(screen.getByRole('button', { name: 'Обновить каталог' })); await screen.findByText(/Данные могут быть устаревшими/); await card()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await waitFor(() => expect(screen.queryByText(/Данные могут быть устаревшими/)).toBeNull())
    setMockScenario('dishes-error'); await user.selectOptions(screen.getByLabelText('Активность'), 'false'); await screen.findByRole('heading', { name: 'Не удалось загрузить каталог блюд' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('shows initial empty and out-of-range recovery', async () => {
    setMockScenario('dishes-empty'); const { user } = mount('/admin/dishes?page=3'); await screen.findByRole('heading', { name: 'На этой странице нет блюд' })
    await user.click(screen.getByRole('button', { name: 'К первой странице' })); await screen.findByRole('heading', { name: 'Каталог блюд пуст' })
  })
  it('refreshes list after Dishes invalidation for a synthetic new recipe version', async () => {
    const { store } = mount(); await card(); setMockScenario('dish-catalog-new-version')
    act(() => { store.dispatch(dishApi.util.invalidateTags(['Dishes'])) })
    await waitFor(async () => expect((await card()).textContent).toContain('Ингредиентов2'))
    expect((await card()).textContent).toContain('Доступные порции7'); expect(requests).toHaveLength(2)
  })
  it.each(['manager', 'storekeeper', 'order_operator'] as const)('denies %s before mounting list requests', async (role) => {
    mount('/admin/dishes', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it.each(['invalid-schema', 'invalid-json'] as const)('rejects malformed %s', async (scenario) => {
    setMockScenario(scenario); mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить каталог блюд' }); expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('rejects mismatched pagination, duplicate IDs and rows outside selected filters', async () => {
    const store = makeStore('admin'); stores.push(store); const dish = dishCatalog()[0]
    for (const body of [{ items: [dish], total: 1, page: 2, page_size: 50 }, { items: [dish, dish], total: 2, page: 1, page_size: 50 }, { items: [dish], total: 1, page: 1, page_size: 50 }]) {
      server.use(http.get('/api/v1/dishes', () => Response.json({ ...body, meta: dish.meta })))
      const request = store.dispatch(dishApi.endpoints.adminDishes.initiate({ ...defaultDishSearch, is_active: body.items.length === 1 && body.page === 1 ? 'false' : 'all' }, { forceRefetch: true }))
      const result = await request; request.unsubscribe(); expect(result.error).toMatchObject({ status: 'INVALID_RESPONSE' })
    }
  })
})
