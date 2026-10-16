// @vitest-environment jsdom
import { StrictMode } from 'react'
import { configureStore } from '@reduxjs/toolkit'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http } from 'msw/http'
import { productApi, productSchema, defaultProductSearch, readProductSearch, productSearchParams, changeProductSearch } from '@/entities/product'
import { categoryApi, createCategorySchema, updateCategorySchema } from '@/entities/product-category'
import { initialSessionState, sessionReducer, sections, saveWorkDraft, type Role } from '@/entities/session'
import { RoleGate } from '@/features/access'
import { safeReturnUrl } from '@/features/auth/model/return-url'
import { catalogHandlers, catalogProductHandlers } from '@/app/mocks/catalog-handlers'
import { catalogProducts, resetCatalogFixtures } from '@/app/mocks/catalog-fixtures'
import { setMockScenario } from '@/app/mocks/scenarios'
import { mockId } from '@/app/mocks/inventory-fixtures'
import { productFields, productToValues } from '@/features/manage-product'
import { inventoryHandlers } from '@/app/mocks/inventory-handlers'
import { procurementHandlers } from '@/app/mocks/procurement-handlers'
import { inventoryApi } from '@/entities/inventory'
import { procurementApi } from '@/entities/procurement'
import { ProductsPage } from './ProductsPage'
import { CategoriesPage } from './CategoriesPage'
import { ProductEntryPage } from './ProductEntryPage'
const server = setupServer(...catalogHandlers, ...catalogProductHandlers, ...inventoryHandlers, ...procurementHandlers), NativeRequest = globalThis.Request
const requests: Request[] = []
function makeStore(role: Role) {
  return configureStore({ reducer: { session: sessionReducer, [productApi.reducerPath]: productApi.reducer },
    preloadedState: { session: { ...initialSessionState, status: 'authenticated' as const, user: { id: mockId(99), name: 'Тест', email: 'test@example.test', role } } },
    middleware: (getDefaultMiddleware) => getDefaultMiddleware({ thunk: { extraArgument: { getCsrfToken: () => 'mock-csrf-for-local-tests' } } }).concat(productApi.middleware) })
}
const stores: ReturnType<typeof makeStore>[] = []
function mount(path = '/admin/products', role: Role = 'admin') {
  const store = makeStore(role); stores.push(store)
  const guard = (id: string, children: React.ReactNode) => <RoleGate section={sections.find((section) => section.id === id)!}>{children}</RoleGate>
  const router = createMemoryRouter([
    { path: '/admin/products', element: guard('products', <ProductsPage />) },
    { path: '/admin/categories', element: guard('categories', <CategoriesPage />) },
    { path: '/admin/products/new', element: guard('products', <ProductEntryPage />) },
    { path: '/admin/products/:id/edit', element: guard('products', <ProductEntryPage />) },
    { path: '/admin/products/:id', element: guard('products', <ProductEntryPage />) },
    { path: '*', element: <h1>Склад</h1> },
  ], { initialEntries: [path] })
  render(<StrictMode><Provider store={store}><RouterProvider router={router} /></Provider></StrictMode>)
  return { store, router, user: userEvent.setup() }
}
const ready = () => screen.findByRole('article', { name: 'Продукт: Молоко' })
beforeAll(() => {
  vi.stubGlobal('Request', class extends NativeRequest { constructor(input: RequestInfo | URL, init?: RequestInit) { super(typeof input === 'string' && input.startsWith('/') ? `${window.location.origin}${input}` : input, init) } })
  HTMLDialogElement.prototype.showModal = function () { this.open = true }; HTMLDialogElement.prototype.close = function () { this.open = false }
  server.listen({ onUnhandledFrame: 'error' }); server.events.on('request:start', ({ request }) => requests.push(request.clone()))
})
beforeEach(() => { setMockScenario('success'); resetCatalogFixtures(); requests.length = 0; document.cookie = 'flagman_mock_role=admin; Path=/' })
afterEach(() => { cleanup(); stores.splice(0).forEach((store) => store.dispatch(productApi.util.resetApiState())); server.resetHandlers() })
afterAll(() => { server.close(); vi.unstubAllGlobals() })
describe('published admin catalogue', () => {
  it('requests explicit all activity and renders configuration rather than stock', async () => {
    mount(); await ready()
    const url = new URL(requests.find((request) => new URL(request.url).pathname === '/api/v1/products')!.url)
    expect(Object.fromEntries(url.searchParams)).toEqual({ is_active: 'all', sort: 'name', page: '1', page_size: '50' })
    const milk = within(screen.getByRole('article', { name: 'Продукт: Молоко' }))
    expect(milk.getByText('4 л')).toBeTruthy(); expect(milk.getByText('48 ч')).toBeTruthy(); expect(milk.getByText('ml')).toBeTruthy()
    expect(screen.queryByText('Годный остаток')).toBeNull()
  })
  it('distinguishes absent target, zero target and positive exact target including disabled products', async () => {
    mount('/admin/products?page_size=100'); await ready()
    expect(within(screen.getByRole('article', { name: 'Продукт: Сахар' })).getByText('0 кг')).toBeTruthy()
    const disabled = screen.getByRole('article', { name: 'Продукт: Творог — только просрочка' })
    expect(within(disabled).getByText('Отключён')).toBeTruthy(); expect(within(disabled).getAllByText('Не задан')).toHaveLength(2)
  })
  it('combines URL filters, resets page and preserves browser history', async () => {
    const { user, router } = mount('/admin/products?page=2&page_size=20'); await screen.findByText('Продуктов: 25. Страница 2.')
    await user.selectOptions(screen.getByLabelText('Активность'), 'false')
    await screen.findByRole('article', { name: 'Продукт: Творог — только просрочка' })
    expect(router.state.location.search).toBe('?is_active=false&page_size=20')
    await user.selectOptions(screen.getByLabelText('Категория'), mockId(2)); await screen.findByRole('heading', { name: 'Ничего не найдено' })
    expect(screen.queryByRole('article')).toBeNull()
    await act(async () => { await router.navigate(-1) }); await screen.findByRole('article', { name: 'Продукт: Творог — только просрочка' })
  })
  it('treats search literally, shows no results and resets filters', async () => {
    const { user } = mount(); await ready(); await user.type(screen.getByLabelText('Поиск продукта'), '%_'); await user.click(screen.getByRole('button', { name: 'Найти' }))
    await screen.findByRole('heading', { name: 'Ничего не найдено' }); expect(screen.queryByRole('article')).toBeNull()
    await user.click(screen.getAllByRole('button', { name: 'Сбросить фильтры' })[0]); await ready()
  })
  it('paginates and recovers an out-of-range page', async () => {
    const { user } = mount('/admin/products?page=9'); await screen.findByRole('heading', { name: 'На этой странице нет продуктов' })
    await user.click(screen.getByRole('button', { name: 'К первой странице' })); await ready()
  })
  it('keeps same-filter stale records, supports retry and hides denied records', async () => {
    const { user, store } = mount(); await ready(); setMockScenario('products-error')
    await act(async () => { store.dispatch(productApi.util.invalidateTags(['Products'])) })
    await screen.findByText(/Данные могут быть устаревшими/); expect(screen.getByRole('article', { name: 'Продукт: Молоко' })).toBeTruthy()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Попробовать снова' })); await waitFor(() => expect(screen.queryByText(/Данные могут быть устаревшими/)).toBeNull())
    setMockScenario('forbidden'); await user.click(screen.getByRole('button', { name: 'Обновить каталог' })); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(screen.queryByRole('article')).toBeNull()
  })
  it('shows empty catalogue and initial error without claiming backend availability', async () => {
    setMockScenario('products-empty'); const { store } = mount(); await screen.findByRole('heading', { name: 'Каталог пуст' })
    setMockScenario('products-error'); await act(async () => { store.dispatch(productApi.util.invalidateTags(['Products'])) }); await screen.findByRole('heading', { name: 'Не удалось загрузить каталог' })
  })
  it('rejects a malformed response instead of crashing quantity formatting', async () => {
    const product = { ...catalogProducts()[0], unit: 'pcs', display_unit: 'pcs', target_stock: '0.001' }
    expect(productSchema.safeParse(product).success).toBe(false)
    server.use(http.get('/api/v1/products', () => Response.json({ items: [product], total: 1, page: 1, page_size: 50, meta: {} })))
    mount(); await screen.findByRole('heading', { name: 'Не удалось загрузить каталог' }); expect(screen.queryByRole('article')).toBeNull()
  })
  it('preserves keyboard links to product entry and warehouse identity', async () => {
    const { user, router } = mount('/admin/products?search=Молоко'); await ready()
    screen.getByRole('link', { name: 'Редактировать: Молоко' }).focus(); await user.keyboard('{Enter}')
    await screen.findByRole('heading', { name: 'Редактирование продукта' }); expect(router.state.location.pathname).toBe(`/admin/products/${mockId(10)}/edit`)
    await user.click(screen.getByRole('link', { name: 'В каталог продуктов' })); await ready(); expect(new URLSearchParams(router.state.location.search).get('search')).toBe('Молоко')
    await user.click(screen.getByRole('link', { name: 'На склад: Молоко' })); expect(new URLSearchParams(router.state.location.search).get('product_id')).toBe(mockId(10))
  })
  for (const role of ['manager', 'storekeeper', 'order_operator'] as const) it(`does not mount either admin screen or requests for ${role}`, async () => {
    const { router } = mount('/admin/products', role); await screen.findByRole('heading', { name: 'Недостаточно прав' }); expect(requests).toHaveLength(0)
    await act(async () => { await router.navigate('/admin/categories') }); expect(requests).toHaveLength(0)
  })
})
describe('category management', () => {
  it('validates blank and Unicode names and revision bounds', () => {
    expect(createCategorySchema.safeParse({ name: '  ' }).success).toBe(false)
    expect(createCategorySchema.safeParse({ name: '😀'.repeat(200) }).success).toBe(true)
    expect(createCategorySchema.safeParse({ name: '😀'.repeat(201) }).success).toBe(false)
    expect(updateCategorySchema.safeParse({ name: 'Имя', expected_revision: 2147483647 }).success).toBe(false)
  })
  it('creates with trimmed published payload and CSRF, refreshes catalogue', async () => {
    const { user, store } = mount(); await ready()
    const catalog = store.dispatch(productApi.endpoints.products.initiate(defaultProductSearch)); await catalog
    await user.click(screen.getByRole('link', { name: 'Управление категориями' })); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    await user.click(screen.getByRole('button', { name: 'Создать категорию' })); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText('Введите название от 1 до 200 символов.'); expect(requests.filter((request) => request.method === 'POST')).toHaveLength(0)
    await user.type(screen.getByLabelText(/Название категории/), '  Овощи  '); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText('Сохранена категория: Овощи.'); await screen.findByRole('article', { name: 'Категория: Овощи' })
    const request = requests.find((request) => request.method === 'POST')!; expect(await request.json()).toEqual({ name: 'Овощи' }); expect(request.headers.get('X-CSRF-Token')).toBe('mock-csrf-for-local-tests')
    await waitFor(() => expect(requests.filter((request) => new URL(request.url).pathname === '/api/v1/products').length).toBeGreaterThan(1)); catalog.unsubscribe()
  })
  it('keeps duplicate rejection editable and never deletes', async () => {
    const { user } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    await user.click(screen.getByRole('button', { name: 'Создать категорию' })); await user.type(screen.getByLabelText(/Название категории/), 'бакалея'); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText('Категория с таким названием уже существует.'); expect((screen.getByLabelText(/Название категории/) as HTMLInputElement).value).toBe('бакалея')
    expect(screen.queryByRole('button', { name: /Удалить/ })).toBeNull(); expect(requests.some((request) => request.method === 'DELETE')).toBe(false)
  })
  it('loads fresh revision, locks conflicts, reloads and requires review before rename', async () => {
    setMockScenario('category-revision-conflict'); const { user } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    await user.click(screen.getByRole('button', { name: 'Переименовать: Бакалея' })); const field = await screen.findByLabelText(/Название категории/)
    await user.clear(field); await user.type(field, 'Крупы'); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText(/Категория изменена другим пользователем/); expect((screen.getByRole('button', { name: 'Сохранить категорию' }) as HTMLButtonElement).disabled).toBe(true)
    expect((field as HTMLInputElement).value).toBe('Крупы')
    await user.click(screen.getByRole('button', { name: 'Загрузить актуальную категорию' })); const fresh = await screen.findByLabelText(/Название категории/)
    expect((fresh as HTMLInputElement).value).toBe('Крупы');
    expect(screen.getByText(/Актуальное название: Бакалея/)).toBeTruthy(); expect((screen.getByRole('button', { name: 'Сохранить категорию' }) as HTMLButtonElement).disabled).toBe(true)
    await user.click(screen.getByRole('button', { name: 'Подтвердить актуальную ревизию' })); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' })); await screen.findByText('Сохранена категория: Крупы.')
    const writes = requests.filter((request) => request.method === 'PATCH'); expect(await writes[0].json()).toEqual({ name: 'Крупы', expected_revision: 1 }); expect(await writes[1].json()).toEqual({ name: 'Крупы', expected_revision: 2 })
  })
  it('preserves dirty editor and expected revision on focus, reconnect and list invalidation', async () => {
    const { user, store } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    await user.click(screen.getByRole('button', { name: 'Переименовать: Бакалея' })); const field = await screen.findByLabelText(/Название категории/)
    await user.clear(field); await user.type(field, 'Моё новое имя')
    const detailReads = () => requests.filter((request) => new URL(request.url).pathname === `/api/v1/product-categories/${mockId(2)}`).length
    const count = detailReads(), listReads = requests.filter((request) => new URL(request.url).pathname === '/api/v1/product-categories').length
    await act(async () => { store.dispatch(categoryApi.internalActions.onFocus()); store.dispatch(categoryApi.internalActions.onOnline()); store.dispatch(categoryApi.util.invalidateTags(['ProductCategories'])) })
    await waitFor(() => expect(requests.filter((request) => new URL(request.url).pathname === '/api/v1/product-categories').length).toBeGreaterThan(listReads))
    expect(detailReads()).toBe(count); expect((screen.getByLabelText(/Название категории/) as HTMLInputElement).value).toBe('Моё новое имя')
    await user.click(screen.getByRole('button', { name: 'Сохранить категорию' })); await screen.findByText('Сохранена категория: Моё новое имя.')
    expect(await requests.find((request) => request.method === 'PATCH')!.json()).toEqual({ name: 'Моё новое имя', expected_revision: 1 })
  })
  it('protects dirty cancellation and route navigation', async () => {
    const { user } = mount('/admin/categories'); await user.click(screen.getByRole('button', { name: 'Создать категорию' })); await user.type(screen.getByLabelText(/Название категории/), 'Новое имя')
    await user.click(screen.getByRole('button', { name: 'Отмена' })); await screen.findByRole('dialog', { name: 'Отменить изменения категории?' })
    await user.click(screen.getByRole('button', { name: 'Продолжить редактирование' })); expect((screen.getByLabelText(/Название категории/) as HTMLInputElement).value).toBe('Новое имя')
    await user.click(screen.getAllByRole('link', { name: 'В каталог продуктов' }).at(-1)!); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' }); await user.click(screen.getByRole('button', { name: 'Остаться' }))
    await user.click(screen.getByRole('button', { name: 'Отмена' })); await user.click(screen.getByRole('button', { name: 'Отменить изменения' })); expect(screen.queryByLabelText(/Название категории/)).toBeNull()
  })
  it('records an indeterminate write without auto-retrying it', async () => {
    setMockScenario('category-lost-response'); const { user } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' }); await user.click(screen.getByRole('button', { name: 'Создать категорию' }))
    await user.type(screen.getByLabelText(/Название категории/), 'Проверить результат'); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText(/Результат сохранения неизвестен/); await screen.findByRole('article', { name: 'Категория: Проверить результат' }); expect(requests.filter((request) => request.method === 'POST')).toHaveLength(1)
  })
  it('filters category reads on the server, supports empty and error states', async () => {
    const { user, store } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' }); await user.type(screen.getByLabelText('Поиск категории'), 'Молоч'); await user.click(screen.getByRole('button', { name: 'Найти категории' }))
    await screen.findByText('Категорий: 1. Страница 1.'); expect(screen.getAllByRole('article')).toHaveLength(1)
    setMockScenario('categories-error'); await act(async () => { store.dispatch(categoryApi.util.invalidateTags(['ProductCategories'])) }); await screen.findByRole('heading', { name: 'Не удалось загрузить категории' })
    expect((screen.getByRole('button', { name: 'Переименовать: Молочные продукты' }) as HTMLButtonElement).disabled).toBe(true)
  })
})
describe('catalogue URL allowlist', () => {
  it('normalizes unsupported parameters and preserves safe admin filters after login', () => {
    expect(readProductSearch(new URLSearchParams('is_active=bad&page=-1&page_size=101&sort=-name&category_id=no'))).toEqual(defaultProductSearch)
    const values = { ...defaultProductSearch, search: 'Молоко', page: 3, is_active: 'false' as const }
    expect(readProductSearch(productSearchParams(values))).toEqual(values)
    expect(changeProductSearch(values, { category_id: mockId(1) }).page).toBe(1)
    expect(safeReturnUrl('/admin/products?search=Молоко&is_active=false&token=secret')).toBe(`/admin/products?${new URLSearchParams({ search: 'Молоко', is_active: 'false' })}`)
  })
})

describe('F18 category review and product draft integration', () => {
  for (const status of [422, 404]) it(`retains proposed rename after ${status}, distinguishes refusal and locks missing writes`, async () => {
    const { user } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    await user.click(screen.getByRole('button', { name: 'Переименовать: Бакалея' })); const field = await screen.findByLabelText(/Название категории/)
    await user.clear(field); await user.type(field, 'Мой черновик'); setMockScenario(status === 422 ? 'category-validation-error' : 'category-not-found')
    await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText(status === 422 ? /Сервер отклонил название/ : /Категория больше не найдена/)
    expect((field as HTMLInputElement).value).toBe('Мой черновик')
    expect((screen.getByRole('button', { name: 'Сохранить категорию' }) as HTMLButtonElement).disabled).toBe(status === 404)
    if (status === 404) { field.focus(); await user.keyboard('{Enter}'); expect(requests.filter((request) => request.method === 'PATCH')).toHaveLength(1) }
  })
  it('retains proposal and old revision lock after a failed conflict reload, never uses stale cached data', async () => {
    setMockScenario('category-revision-conflict'); const { user } = mount('/admin/categories')
    await screen.findByRole('article', { name: 'Категория: Бакалея' }); await user.click(screen.getByRole('button', { name: 'Переименовать: Бакалея' }))
    const field = await screen.findByLabelText(/Название категории/); await user.clear(field); await user.type(field, 'Предложенное имя'); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByText(/Категория изменена другим пользователем/); setMockScenario('categories-error')
    await user.click(screen.getByRole('button', { name: 'Загрузить актуальную категорию' })); await screen.findByText(/Не удалось загрузить актуальную категорию/)
    expect((field as HTMLInputElement).value).toBe('Предложенное имя'); expect(screen.queryByRole('button', { name: 'Подтвердить актуальную ревизию' })).toBeNull()
    setMockScenario('success'); await user.click(screen.getByRole('button', { name: 'Загрузить актуальную категорию' })); await screen.findByText(/Актуальное название: Бакалея \(обновлено\)/)
    field.focus(); await user.keyboard('{Enter}'); expect(requests.filter((request) => request.method === 'PATCH')).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Подтвердить актуальную ревизию' })); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' })); await screen.findByText('Сохранена категория: Предложенное имя.')
    expect(await requests.filter((request) => request.method === 'PATCH')[1].json()).toEqual({ name: 'Предложенное имя', expected_revision: 2 })
  })
  it('returns focus after create and rename and obtains a fresh revision when reopening', async () => {
    const { user } = mount('/admin/categories'); await screen.findByRole('article', { name: 'Категория: Бакалея' })
    const opener = screen.getByRole('button', { name: 'Создать категорию' }); await user.click(opener); await user.click(screen.getByRole('button', { name: 'Отмена' })); await waitFor(() => expect(document.activeElement).toBe(opener))
    const rename = screen.getByRole('button', { name: 'Переименовать: Бакалея' }); await user.click(rename); const field = await screen.findByLabelText(/Название категории/)
    await user.clear(field); await user.type(field, 'Крупы'); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' })); await screen.findByText('Сохранена категория: Крупы.')
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Переименовать: Крупы' })))
    await user.click(screen.getByRole('button', { name: 'Переименовать: Крупы' })); await screen.findByLabelText(/Название категории/); expect(screen.getByText(/уникальность проверяет сервер. Ревизия: 2/)).toBeTruthy()
  })
  it('creates and selects a category without losing local product name or sending any product write', async () => {
    const { user, store } = mount('/admin/products/new'); await screen.findByRole('option', { name: 'Бакалея' })
    await user.type(screen.getByLabelText('Название продукта'), 'Новый продукт'); await user.selectOptions(screen.getByLabelText('Категория продукта'), mockId(2))
    const choices = store.dispatch(categoryApi.endpoints.categories.initiate({ search: '', sort: 'name', page: 1, page_size: 50 })); await choices
    const opener = screen.getByRole('button', { name: 'Создать категорию для продукта' }); await user.click(opener)
    await screen.findByRole('dialog', { name: 'Создание категории' }); await user.type(screen.getByLabelText(/Название категории/), '  Овощи из продукта  '); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' }))
    await screen.findByRole('option', { name: 'Овощи из продукта' }); expect((screen.getByLabelText('Название продукта') as HTMLInputElement).value).toBe('Новый продукт')
    const saved = await requests.find((request) => request.method === 'POST')!.json(); expect(saved).toEqual({ name: 'Овощи из продукта' })
    expect((screen.getByLabelText('Категория продукта') as HTMLSelectElement).selectedOptions[0].text).toBe('Овощи из продукта')
    await waitFor(() => expect(document.activeElement).toBe(opener)); expect(requests.filter((request) => request.method !== 'GET' && !request.url.includes('/product-categories'))).toHaveLength(0)
    const refreshed = await store.dispatch(categoryApi.endpoints.categories.initiate({ search: '', sort: 'name', page: 1, page_size: 50 }, { subscribe: false })).unwrap(); expect(refreshed.items.some((category) => category.name === 'Овощи из продукта')).toBe(true); choices.unsubscribe()
  })
  it('protects dirty modal cancellation and retains product category on discard', async () => {
    const { user } = mount('/admin/products/new'); await screen.findByRole('option', { name: 'Бакалея' }); await user.type(screen.getByLabelText('Название продукта'), 'Сохранить имя'); await user.selectOptions(screen.getByLabelText('Категория продукта'), mockId(2))
    const opener = screen.getByRole('button', { name: 'Создать категорию для продукта' }); await user.click(opener); await user.type(screen.getByLabelText(/Название категории/), 'Не сохранять')
    await user.click(within(screen.getByRole('dialog', { name: 'Создание категории' })).getByRole('button', { name: 'Закрыть' })); await screen.findByRole('dialog', { name: 'Отменить изменения категории?' })
    await user.click(screen.getByRole('button', { name: 'Продолжить редактирование' })); expect((screen.getByLabelText(/Название категории/) as HTMLInputElement).value).toBe('Не сохранять')
    await user.click(screen.getByRole('button', { name: 'Отмена' })); await user.click(screen.getByRole('button', { name: 'Отменить изменения' }))
    expect((screen.getByLabelText('Категория продукта') as HTMLSelectElement).value).toBe(mockId(2)); expect((screen.getByLabelText('Название продукта') as HTMLInputElement).value).toBe('Сохранить имя'); await waitFor(() => expect(document.activeElement).toBe(opener))
  })
})

describe('F17 product creation and revision editing', () => {
  const milkId = mockId(10), edit = `/admin/products/${milkId}/edit`
  const name = () => screen.getByLabelText('Название продукта') as HTMLInputElement
  const target = () => screen.getByLabelText('Целевой запас') as HTMLInputElement
  const save = () => screen.getByRole('button', { name: 'Сохранить продукт' })
  const reload = () => screen.getByRole('button', { name: 'Загрузить актуальный продукт' })
  const writes = () => requests.filter((request) => request.method !== 'GET' && new URL(request.url).pathname.startsWith('/api/v1/products'))
  it('validates names/category/lifetime/exact targets before any write, with field focus', async () => {
    const { user } = mount('/admin/products/new'); await screen.findByRole('option', { name: 'Бакалея' }); await user.click(save())
    await screen.findByText(/Название: от 1 до 200/); expect(document.activeElement).toBe(name()); expect(writes()).toHaveLength(0)
    await user.type(name(), 'Черновик'); await user.selectOptions(screen.getByLabelText('Категория продукта'), mockId(2)); await user.type(screen.getByLabelText('Стандартный срок хранения, часы'), '0'); await user.type(target(), '-1')
    await user.click(save()); await screen.findByText(/Срок: целое число от 1/); expect(target().getAttribute('aria-invalid')).toBe('true'); expect(writes()).toHaveLength(0)
  })
  it('creates trimmed Unicode product with exact zero, clears draft and preserves return filters', async () => {
    const { user, store, router } = mount('/admin/products/new?returnTo=%2Fadmin%2Fproducts%3Fis_active%3Dfalse'); await screen.findByRole('option', { name: 'Бакалея' })
    await user.type(name(), '  Свежая зелень 😀  '); await user.selectOptions(screen.getByLabelText('Категория продукта'), mockId(2)); await user.type(target(), '0'); await user.click(screen.getByLabelText('Продукт активен')); await user.click(save())
    await screen.findByRole('heading', { name: 'Продукт сохранён: Свежая зелень 😀' }); expect(store.getState().session.drafts['product:new']).toBeUndefined()
    expect(await writes()[0].json()).toEqual({ name: 'Свежая зелень 😀', category_id: mockId(2), unit: 'g', display_unit: 'g', standard_shelf_life_hours: null, target_stock: '0.000', is_active: false })
    await user.click(screen.getByRole('link', { name: 'Вернуться в каталог' })); await screen.findByRole('article', { name: 'Продукт: Свежая зелень 😀' }); expect(router.state.location.search).toBe('?is_active=false')
  })
  it('sends only changed PATCH fields and refreshes catalogue/inventory/procurement without changing batches', async () => {
    const { user, store } = mount(edit); await screen.findByDisplayValue('Молоко')
    const catalogue = store.dispatch(productApi.endpoints.products.initiate(defaultProductSearch)); await catalogue
    const inventory = store.dispatch(inventoryApi.endpoints.inventory.initiate({ search: '', category_id: '', filter: 'all', sort: 'name', page: 1, page_size: 50 })); await inventory
    const procurement = store.dispatch(procurementApi.endpoints.procurement.initiate({ search: '', category_id: '', page: 1, page_size: 50 })); await procurement
    const batches = await store.dispatch(inventoryApi.endpoints.productBatches.initiate({ productId: milkId, page: 1, page_size: 50, status: 'all', sort: 'expires_at' }, { subscribe: false })).unwrap()
    const counts = () => requests.filter((request) => /\/api\/v1\/(inventory|procurement-recommendations|products)$/.test(new URL(request.url).pathname)).length
    const count = counts(); await user.clear(target()); await user.type(target(), '9999,999'); await user.clear(screen.getByLabelText('Стандартный срок хранения, часы')); await user.type(screen.getByLabelText('Стандартный срок хранения, часы'), '72'); await user.click(save())
    await screen.findByRole('heading', { name: 'Продукт сохранён: Молоко' }); expect(await writes()[0].json()).toEqual({ expected_revision: 1, target_stock: '9999.999', standard_shelf_life_hours: 72 })
    await waitFor(() => expect(counts()).toBeGreaterThan(count + 2))
    const after = await store.dispatch(inventoryApi.endpoints.productBatches.initiate({ productId: milkId, page: 1, page_size: 50, status: 'all', sort: 'expires_at' }, { subscribe: false, forceRefetch: true })).unwrap(); expect(after).toEqual(batches)
    const current = await store.dispatch(productApi.endpoints.products.initiate(defaultProductSearch, { subscribe: false })).unwrap(); expect(current.items.find((product) => product.id === milkId)?.standard_shelf_life_hours).toBe(72)
    catalogue.unsubscribe(); inventory.unsubscribe(); procurement.unsubscribe()
  })
  it('sends explicit null without including unrelated fields', async () => {
    const { user } = mount(edit); await screen.findByDisplayValue('Молоко'); await user.clear(target()); await user.clear(screen.getByLabelText('Стандартный срок хранения, часы')); await user.click(save())
    await screen.findByRole('heading', { name: 'Продукт сохранён: Молоко' }); expect(await writes()[0].json()).toEqual({ expected_revision: 1, target_stock: null, standard_shelf_life_hours: null })
  })
  it('preserves all proposal fields on conflict/failed reload, requires review and uses the fresh revision', async () => {
    setMockScenario('product-revision-conflict'); const { user, store } = mount(edit); await screen.findByDisplayValue('Молоко')
    await user.clear(name()); await user.type(name(), 'Моё молоко'); await user.clear(target()); await user.type(target(), '7000.001'); await user.click(save()); await screen.findByText(/Продукт изменён другим пользователем/)
    expect((save() as HTMLButtonElement).disabled).toBe(true); setMockScenario('products-error'); await user.click(reload()); await screen.findByText(/Не удалось загрузить актуальный продукт/); expect(name().value).toBe('Моё молоко'); expect(target().value).toBe('7000.001')
    setMockScenario('success'); await user.click(reload()); await screen.findByRole('heading', { name: /Актуальный продукт: Молоко \(обновлено\). Ревизия: 2/ }); expect(target().value).toBe('7000.001')
    const reads = () => requests.filter((request) => new URL(request.url).pathname === `/api/v1/products/${milkId}` && request.method === 'GET').length
    const count = reads(); await act(async () => { store.dispatch(productApi.internalActions.onFocus()); store.dispatch(productApi.internalActions.onOnline()); store.dispatch(productApi.util.invalidateTags(['Products'])) }); expect(reads()).toBe(count)
    name().focus(); await user.keyboard('{Enter}'); expect(writes()).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Подтвердить актуальную ревизию' })); await user.click(save()); await screen.findByRole('heading', { name: 'Продукт сохранён: Моё молоко' }); expect(await writes()[1].json()).toEqual({ expected_revision: 2, name: 'Моё молоко', target_stock: '7000.001' })
  })
  for (const scenario of ['product-unit-locked', 'product-in-use', 'product-validation-error', 'product-not-found', 'conflict'] as const) it(`retains input on ${scenario}`, async () => {
    const { user } = mount(edit); await screen.findByDisplayValue('Молоко'); await user.clear(name()); await user.type(name(), 'Сохранить предложение'); await user.clear(target()); await user.type(target(), '4000.001')
    if (scenario === 'product-unit-locked') await user.selectOptions(screen.getByLabelText('Базовая единица'), 'g')
    if (scenario === 'product-in-use') await user.click(screen.getByLabelText('Продукт активен'))
    if (scenario === 'conflict') { await user.clear(name()); await user.type(name(), 'Сахар') } else setMockScenario(scenario)
    await user.click(save())
    await screen.findByText(scenario === 'product-unit-locked' ? /Базовую единицу нельзя изменить/ : scenario === 'product-in-use' ? /Продукт нельзя отключить/ : scenario === 'product-validation-error' ? /Сервер отклонил поля/ : scenario === 'product-not-found' ? /Продукт или категория не найдены/ : /Продукт с таким названием уже существует/)
    expect(name().value).toBe(scenario === 'conflict' ? 'Сахар' : 'Сохранить предложение'); expect(target().value).toBe('4000.001')
    if (scenario === 'product-validation-error') expect(target().getAttribute('aria-invalid')).toBe('true')
    if (scenario === 'product-in-use') expect(screen.getByText('Демонстрационное активное блюдо')).toBeTruthy()
    if (scenario === 'product-not-found') expect((save() as HTMLButtonElement).disabled).toBe(true)
  })
  it('retains unknown outcomes, blocks replay, explicitly reloads/reviews a lost PATCH result', async () => {
    setMockScenario('product-lost-response'); const { user, store } = mount(edit); await screen.findByDisplayValue('Молоко'); await user.clear(name()); await user.type(name(), 'Ответ потерян'); await user.click(save())
    await screen.findByText(/Результат сохранения неизвестен/); expect((save() as HTMLButtonElement).disabled).toBe(true); expect(store.getState().session.drafts[`product:${milkId}`].product?.uncertain).toBe(true)
    name().focus(); await user.keyboard('{Enter}'); expect(writes()).toHaveLength(1); setMockScenario('success'); await user.click(reload()); await screen.findByRole('heading', { name: /Актуальный продукт: Ответ потерян. Ревизия: 2/ }); await user.click(screen.getByRole('button', { name: 'Использовать актуальные поля' })); expect(writes()).toHaveLength(1); expect(store.getState().session.drafts[`product:${milkId}`]).toBeUndefined()
  })
  it('restores dirty edit against a fresh changed revision and protects departure/discard', async () => {
    const { user, store, router } = mount('/admin/products')
    const product = catalogProducts().find((item) => item.id === milkId)!, fields = productFields(productToValues(product))
    store.dispatch(saveWorkDraft({ key: `product:${milkId}`, draft: { product: { resourceId: milkId, values: { ...productToValues(product), name: 'Мой сохранённый черновик' }, baseline: { fields, revision: 2 }, uncertain: false, review: false } } }))
    await act(async () => { await router.navigate(edit) }); await screen.findByDisplayValue('Мой сохранённый черновик'); expect((save() as HTMLButtonElement).disabled).toBe(true)
    await user.click(screen.getByRole('link', { name: 'В каталог продуктов' })); await screen.findByRole('dialog', { name: 'Уйти без сохранения?' }); await user.click(screen.getByRole('button', { name: 'Остаться' })); expect(name().value).toBe('Мой сохранённый черновик')
    await user.click(screen.getByRole('link', { name: 'В каталог продуктов' })); await user.click(screen.getByRole('button', { name: 'Уйти без сохранения' })); await ready(); expect(store.getState().session.drafts[`product:${milkId}`]).toBeUndefined()
  })
  it('accepts a created category at the published 200 Unicode-character boundary', async () => {
    const { user } = mount('/admin/products/new'); await screen.findByRole('option', { name: 'Бакалея' }); await user.type(name(), 'Юникодный продукт')
    await user.click(screen.getByRole('button', { name: 'Создать категорию для продукта' })); const categoryName = '😀'.repeat(200)
    await user.type(screen.getByLabelText(/Название категории/), categoryName); await user.click(screen.getByRole('button', { name: 'Сохранить категорию' })); await screen.findByRole('option', { name: categoryName })
    await waitFor(() => expect((save() as HTMLButtonElement).disabled).toBe(false)); await user.click(save()); await screen.findByRole('heading', { name: 'Продукт сохранён: Юникодный продукт' }); expect(writes()).toHaveLength(1)
  })
  it('shows empty category choices and allows category creation; failed choices preserve the form', async () => {
    setMockScenario('categories-empty'); const { user, store } = mount('/admin/products/new'); await screen.findByRole('heading', { name: 'Категорий пока нет' })
    expect((save() as HTMLButtonElement).disabled).toBe(true); expect((screen.getByRole('button', { name: 'Создать категорию для продукта' }) as HTMLButtonElement).disabled).toBe(false)
    await user.type(name(), 'Сохранить при ошибке категорий'); setMockScenario('categories-error'); await act(async () => { store.dispatch(inventoryApi.util.invalidateTags(['ProductCategories'])) })
    await screen.findByRole('heading', { name: 'Не удалось загрузить варианты категорий' }); expect(name().value).toBe('Сохранить при ошибке категорий'); expect((save() as HTMLButtonElement).disabled).toBe(true)
  })
  it('treats wrong returned fields/revision as unknown, without clearing or retrying the draft', async () => {
    const product = catalogProducts().find((item) => item.id === milkId)!
    server.use(http.patch('/api/v1/products/:id', () => Response.json({ ...product, revision: 1 })))
    const { user, store } = mount(edit); await screen.findByDisplayValue('Молоко'); await user.clear(name()); await user.type(name(), 'Подтвердить на сервере'); await user.click(save())
    await screen.findByText(/Результат сохранения неизвестен/); expect(name().value).toBe('Подтвердить на сервере'); expect((save() as HTMLButtonElement).disabled).toBe(true); expect(writes()).toHaveLength(1); expect(store.getState().session.drafts[`product:${milkId}`].product?.uncertain).toBe(true)
  })
  for (const status of [404, 403]) it(`hides a previously cached product on a fresh ${status} read`, async () => {
    const { store, router } = mount('/admin/products'); await ready()
    await store.dispatch(productApi.endpoints.product.initiate(milkId, { subscribe: false })).unwrap()
    setMockScenario(status === 404 ? 'product-not-found' : 'forbidden')
    await act(async () => { await router.navigate(edit) })
    await screen.findByRole('heading', { name: status === 404 ? 'Продукт не найден' : 'Недостаточно прав' }); expect(screen.queryByLabelText('Название продукта')).toBeNull()
  })
})
