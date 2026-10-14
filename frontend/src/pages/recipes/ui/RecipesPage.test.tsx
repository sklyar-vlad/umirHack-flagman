// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { dishApi, safeRecipeListUrl } from '@/entities/dish'
import { initialSessionState, sessionReducer, sections, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { orderCompositionHandlers } from '@/app/mocks/order-composition-handlers'
import { compositionDishes, resetCompositionFixtures } from '@/app/mocks/order-composition-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { RecipesPage } from './RecipesPage'
import { RecipeDetailPage } from './RecipeDetailPage'
const server = setupServer(...orderCompositionHandlers), NativeRequest = globalThis.Request
const requests: Request[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [dishApi.reducerPath]: dishApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: compositionDishes()[0].id, name: 'Тест', email: 'test@example.test', role } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(dishApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/dishes', role: Role = 'manager') {
  const store = makeStore(role); stores.push(store)
  const guard = (children: React.ReactNode) => <RoleGate section={sections.find((section) => section.id === 'dishes')!}>{children}</RoleGate>
  const router = createMemoryRouter([
    { path: '/dishes', element: guard(<RecipesPage />) },
    { path: '/dishes/:id', element: guard(<RecipeDetailPage />) },
  ], { initialEntries: [path] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, router, user: userEvent.setup() }
}
const soup = () => screen.findByRole('article', { name: 'Рецептура: Суп дня' })
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request))
})
beforeEach(() => { setMockScenario('success'); resetCompositionFixtures(); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dishApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
describe('recipe reading and navigation', () => {
  it.each(['admin', 'manager', 'order_operator'] as const)('reads catalogue as %s without writes or reservations', async (role) => {
    mount('/dishes', role)
    expect((await soup()).textContent).toContain('100 мл')
    expect(screen.queryByRole('link', { name: 'Управлять рецептурами' }) !== null).toBe(role === 'admin')
    expect((await screen.findByRole('article', { name: 'Рецептура: Без рецептуры' })).textContent).toContain('Состав пока не задан')
    expect(requests).toHaveLength(1); expect(requests[0].method).toBe('GET')
  })
  it('blocks storekeeper before requesting the catalogue', async () => {
    mount('/dishes', 'storekeeper'); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
  })
  it('keeps filters on return from exact recipe details and on authentication recovery', async () => {
    const { user, router } = mount('/dishes?search=Суп')
    await user.click(within(await soup()).getByRole('link', { name: 'Открыть рецептуру' }))
    await screen.findByRole('heading', { name: 'Суп дня' })
    expect(screen.getByRole('region', { name: 'Состав на одну порцию' }).textContent).toContain('100 мл')
    await user.click(screen.getByRole('link', { name: /К рецептурам/ }))
    await soup(); expect(router.state.location.search).toBe('?search=%D0%A1%D1%83%D0%BF')
    expect(safeReturnUrl('/dishes?search=Суп&token=secret')).toBe('/dishes?search=%D0%A1%D1%83%D0%BF')
    const detail = `/dishes/${compositionDishes()[0].id}`
    expect(safeReturnUrl(`${detail}?${new URLSearchParams({ returnTo: '/dishes?search=Суп&password=secret', token: 'secret' })}`)).toBe(`${detail}?${new URLSearchParams({ returnTo: '/dishes?search=%D0%A1%D1%83%D0%BF' })}`)
    expect(safeRecipeListUrl('//external.test')).toBe('/dishes')
    expect(safeRecipeListUrl('/admin/users')).toBe('/dishes')
    expect(requests.every((request) => request.method === 'GET')).toBe(true)
  })
  it('filters, resets pagination and reports no results', async () => {
    const { user, router } = mount('/dishes?page=2&page_size=1')
    await screen.findByRole('article', { name: 'Рецептура: Отключённое блюдо' })
    await user.selectOptions(screen.getByLabelText('Активность'), 'false')
    expect(router.state.location.search).not.toContain('page=')
    await screen.findByRole('article', { name: 'Рецептура: Отключённое блюдо' })
    await user.type(screen.getByLabelText('Поиск блюда'), '%_')
    await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByRole('heading', { name: 'Ничего не найдено' })
    expect(screen.queryAllByRole('article')).toHaveLength(0)
  })
  it('keeps cached list on refresh failure and recovers with retry', async () => {
    const { user } = mount(); await soup(); setMockScenario('dishes-error')
    await user.click(screen.getByRole('button', { name: 'Обновить рецептуры' }))
    await screen.findByRole('heading', { name: 'Не удалось загрузить рецептуры' }); await soup()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })
  it('updates precise quantities after a Dishes event without hiding loaded data', async () => {
    const { store } = mount(); await soup(); setMockScenario('dish-catalog-new-version')
    act(() => { store.dispatch(dishApi.util.invalidateTags(['Dishes'])) })
    await waitFor(async () => expect((await soup()).textContent).toContain('0,001 г'))
    expect(screen.queryByRole('status', { name: 'Загрузка рецептур' })).toBeNull()
  })
  it.each(['dish-detail-error', 'dish-detail-missing', 'forbidden'] as const)('shows detail state for %s', async (scenario) => {
    setMockScenario(scenario); mount(`/dishes/${compositionDishes()[0].id}`)
    await screen.findByRole('heading', { name: scenario === 'dish-detail-error' ? 'Не удалось загрузить рецептуру' : scenario === 'dish-detail-missing' ? 'Блюдо не найдено' : 'Недостаточно прав' })
    expect(screen.queryByRole('region', { name: 'Состав на одну порцию' })).toBeNull()
  })
  it('handles invalid route IDs without issuing an API request', async () => {
    mount('/dishes/not-an-id'); await screen.findByRole('heading', { name: 'Блюдо не найдено' }); expect(requests).toHaveLength(0)
  })
})
