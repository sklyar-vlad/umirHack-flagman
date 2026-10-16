// @vitest-environment jsdom
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { delay } from 'msw'
import { initialSessionState, sessionReducer, saveWorkDraft } from '@/entities/session'
import { dishApi, dishChangesReducer, defaultDishSearch } from '@/entities/dish'
import { handlers } from '@/app/mocks/handlers'
import { dishCatalog } from '@/app/mocks/dish-catalog-fixtures'
import { editorPreview, resetDishEditorFixtures } from '@/app/mocks/dish-editor-fixtures'
import { resetCatalogFixtures } from '@/app/mocks/catalog-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { emptyDish, dishBaseline, dishToValues } from '@/features/manage-dish'
import { DishForm } from './DishForm'
const server = setupServer(...handlers), NativeRequest = globalThis.Request
const requests: Request[] = [], stores: ReturnType<typeof makeStore>[] = []
const milk = '00000000-0000-4000-8000-000000000010'
function makeStore() {
  return configureStore({ reducer: { session: sessionReducer, dishChanges: dishChangesReducer, [dishApi.reducerPath]: dishApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, draftOwner: milk, user: { id: milk, name: 'Тест', email: 'test@example.test', role: 'admin' as const } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests' } } }).concat(dishApi.middleware) })
}
function mount(edit = false, store = makeStore(), onReload = vi.fn(async () => dishCatalog()[0]), initialDish = dishCatalog()[0]) {
  stores.push(store)
  const router = createMemoryRouter([{ path: '/', element: <DishForm dish={edit ? initialDish : undefined} returnTo="/admin/dishes" onReload={onReload} /> }], { initialEntries: ['/'] })
  render(<Provider store={store}><RouterProvider router={router} /></Provider>)
  return { store, user: userEvent.setup(), onReload }
}
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request.clone()))
})
beforeEach(() => { sessionStorage.clear(); document.cookie = 'flagman_mock_role=admin'; resetCatalogFixtures(); resetDishEditorFixtures(); setMockScenario('success'); requests.length = 0 })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(dishApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
const previewButton = () => screen.getByRole('button', { name: 'Проверить без сохранения' })
const saveButton = (edit = false) => screen.getByRole('button', { name: edit ? 'Сохранить изменения блюда' : 'Создать блюдо' })
async function fill(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() => expect(screen.getByLabelText(/^Продукт 1/).textContent).toContain('Молоко'))
  await user.selectOptions(screen.getByLabelText(/^Продукт 1/), milk); await user.type(screen.getByLabelText(/^Расход 1/), '100')
}
describe('F20 editor behavior using explicit MSW only', () => {
  it('an untouched missing recipe has no dirty changes and explains the 1–100 requirement', async () => {
    const dish = dishCatalog().find((dish) => dish.recipe_version === null)!
    const { user, store } = mount(true, makeStore(), undefined, dish)
    expect((saveButton(true) as HTMLButtonElement).disabled).toBe(true)
    await waitFor(() => expect((previewButton() as HTMLButtonElement).disabled).toBe(false))
    await user.click(previewButton()); await screen.findByText('Добавьте от 1 до 100 ингредиентов')
    expect(store.getState().session.drafts[`dish:${dish.id}`]).toBeUndefined()
    await user.click(screen.getByRole('button', { name: 'Добавить ингредиент' })); expect(screen.getByLabelText(/^Продукт 1/)).toBeTruthy()
  })
  it('renders server expiration-only shortage without client stock arithmetic', async () => {
    server.use(http.post('/api/v1/dishes/preview', () => {
      const result = editorPreview([{ product_id: milk, quantity_per_portion: '100.000' }])
      const line = { ...result.requirements[0], available_quantity: '0.000', missing_quantity: '100.000' }
      return Response.json({ ...result, is_available: false, available_portions: '0', requirements: [line], shortages: [{ ...line, expired_quantity: '500.000', reason_code: 'only_expired_stock' }] })
    }))
    const { user } = mount(); await fill(user); await user.click(previewButton())
    const result = await screen.findByRole('region', { name: 'Проверка рецептуры' }); expect(result.textContent).toContain('Осталась только просрочка'); expect(result.textContent).toContain('просрочено 500 мл'); expect(result.textContent).toContain('Доступные порции: 0')
  })
  it('preview without name has no idempotency key, no writes and preserves exact one-portion requirements', async () => {
    const { user } = mount(); await fill(user); await user.click(previewButton())
    const result = await screen.findByRole('region', { name: 'Проверка рецептуры' }); expect(result.textContent).toContain('Доступные порции: 15'); expect(result.textContent).toContain('требуется 100 мл')
    const writes = requests.filter((request) => request.method === 'POST'); expect(writes).toHaveLength(1); expect(new URL(writes[0].url).pathname).toBe('/api/v1/dishes/preview'); expect(writes[0].headers.get('Idempotency-Key')).toBeNull()
    await user.type(screen.getByLabelText(/^Название блюда/), 'Имя'); expect(screen.queryByRole('region', { name: 'Проверка рецептуры' })).toBeNull()
  })
  it('discards delayed preview after input changes even when values are restored', async () => {
    server.use(http.post('/api/v1/dishes/preview', async () => { await delay(250); return Response.json(editorPreview([{ product_id: milk, quantity_per_portion: '100.000' }])) }))
    const { user } = mount(); await fill(user); await user.click(previewButton()); await user.clear(screen.getByLabelText(/^Расход 1/)); await user.type(screen.getByLabelText(/^Расход 1/), '100')
    await new Promise((resolve) => setTimeout(resolve, 350)); expect(screen.queryByRole('region', { name: 'Проверка рецептуры' })).toBeNull(); expect(screen.getByText(/Предыдущая проверка неактуальна/)).toBeTruthy()
  })
  it('save clears draft only on confirmed matching result and refreshes active dependent reads', async () => {
    const { store, user } = mount(); await fill(user); await user.type(screen.getByLabelText(/^Название блюда/), 'Новая запись')
    const subscriptions = [store.dispatch(dishApi.endpoints.adminDishes.initiate(defaultDishSearch)), store.dispatch(dishApi.endpoints.stopList.initiate({ search: '', page: 1, page_size: 50 })), store.dispatch(dishApi.endpoints.hotOffers.initiate({ search: '', page: 1, page_size: 50 }))]
    await Promise.all(subscriptions); await user.click(saveButton()); await screen.findByRole('heading', { name: 'Блюдо сохранено: Новая запись' })
    expect(store.getState().session.drafts['dish:new']).toBeUndefined(); expect(store.getState().dishChanges).toBe(1)
    await waitFor(() => expect(requests.filter((request) => new URL(request.url).pathname === '/api/v1/dishes' && request.method === 'GET')).toHaveLength(2))
    for (const path of ['/stop-list', '/hot-offers']) await waitFor(() => expect(requests.filter((request) => new URL(request.url).pathname.endsWith(path))).toHaveLength(2))
    subscriptions.forEach((request) => request.unsubscribe())
  })
  it.each(['dish-validation', 'dish-duplicate'] as const)('retains draft and focuses field after %s', async (scenario) => {
    const { store, user } = mount(); await fill(user); await user.type(screen.getByLabelText(/^Название блюда/), 'Моя запись'); setMockScenario(scenario); await user.click(saveButton())
    await screen.findByText(scenario === 'dish-validation' ? 'Сервер: проверьте расход' : 'Сервер: повтор ингредиента')
    expect(document.activeElement).toBe(screen.getByLabelText(scenario === 'dish-validation' ? /^Расход 1/ : /^Продукт 1/)); expect(store.getState().session.drafts['dish:new'].dish?.values.name).toBe('Моя запись')
  })
  it('requires explicit conflict reload and review; keeps all proposed fields', async () => {
    const { user } = mount(true); await user.clear(screen.getByLabelText(/^Название блюда/)); await user.type(screen.getByLabelText(/^Название блюда/), 'Моя версия'); setMockScenario('dish-revision-conflict'); await user.click(saveButton(true)); await screen.findByText(/Блюдо изменено другим пользователем/)
    expect((saveButton(true) as HTMLButtonElement).disabled).toBe(true); await user.click(screen.getByRole('button', { name: 'Загрузить актуальную версию блюда' })); const panel = await screen.findByRole('region', { name: 'Проверка актуальной версии' }); expect(panel.textContent).toContain('Суп дня (обновлено)')
    expect((screen.getByLabelText(/^Название блюда/) as HTMLInputElement).value).toBe('Моя версия'); expect((saveButton(true) as HTMLButtonElement).disabled).toBe(true)
    await user.click(within(panel).getByRole('button', { name: 'Подтверждаю проверку всех полей блюда' })); await user.click(saveButton(true)); await screen.findByRole('heading', { name: 'Блюдо сохранено: Моя версия' })
    expect((await requests.filter((request) => request.method === 'PATCH').at(-1)!.clone().json()).expected_revision).toBe(2)
  })
  it('restores an owner draft; malformed success stays uncertain without automatic replay', async () => {
    const store = makeStore(), draft = { resourceId: 'new' as const, values: { ...emptyDish, name: 'Восстановлено', recipe: [{ product_id: milk, quantity_per_portion: '100', unit: 'ml' as const }] }, baseline: null, uncertain: false, review: false }
    store.dispatch(saveWorkDraft({ key: 'dish:new', draft: { dish: draft } })); server.use(http.post('/api/v1/dishes', () => Response.json({ ...dishCatalog()[0], name: 'Неправильный ответ' }, { status: 201 })))
    const { user } = mount(false, store); await waitFor(() => expect((saveButton() as HTMLButtonElement).disabled).toBe(false)); await user.click(saveButton()); await screen.findByText(/Результат сохранения неизвестен/)
    expect(store.getState().session.drafts['dish:new'].dish?.uncertain).toBe(true); expect((saveButton() as HTMLButtonElement).disabled).toBe(true); await user.click(saveButton()); expect(requests.filter((request) => request.method === 'POST')).toHaveLength(1)
  })
  it('rejects unrelated preview ingredient and a PATCH success with unchanged recipe version', async () => {
    const { store } = mount(true); const original = dishCatalog()[0], body = { recipe: [{ product_id: milk, quantity_per_portion: '101.000' }] }
    server.use(http.post('/api/v1/dishes/preview', () => Response.json(editorPreview([{ product_id: milk, quantity_per_portion: '100.000' }]))))
    const preview = await store.dispatch(dishApi.endpoints.previewDish.initiate(body)); expect(preview.error).toMatchObject({ status: 'INVALID_RESPONSE' })
    server.use(http.post('/api/v1/dishes/preview', () => Response.json({ ...editorPreview(body.recipe), requirements: [{ ...editorPreview(body.recipe).requirements[0], required_quantity: '100000000000000000000.000' }] })))
    const oversized = await store.dispatch(dishApi.endpoints.previewDish.initiate(body)); expect(oversized.error).toMatchObject({ status: 'INVALID_RESPONSE' })
    server.use(http.patch('/api/v1/dishes/:id', () => Response.json({ ...original, revision: 2, recipe: [{ ...original.recipe[0], quantity_per_portion: '101.000' }] })))
    const result = await store.dispatch(dishApi.endpoints.updateDish.initiate({ id: original.id, body: { expected_revision: 1, ...body }, previousVersion: original.recipe_version })); expect(result.error).toMatchObject({ status: 'INVALID_RESPONSE' })
    expect(dishBaseline(original).revision).toBe(1); expect(dishToValues(original).recipe[0].quantity_per_portion).toBe('100')
  })
})
