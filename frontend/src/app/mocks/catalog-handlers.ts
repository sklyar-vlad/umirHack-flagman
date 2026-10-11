import { delay } from 'msw'
import { http } from 'msw/http'
import { createCategorySchema, updateCategorySchema } from '@/entities/product-category'
import { catalogCategories, catalogProducts, saveCatalogCategory } from './catalog-fixtures'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'
async function readError(scope: 'products' | 'categories') {
  const scenario = getMockScenario()
  if (scenario === 'slow' || scope === 'products' && scenario === 'products-slow') await delay(2000)
  if (scope === 'products' && scenario === 'products-error' || scope === 'categories' && scenario === 'categories-error') return createMockError(500, 'INTERNAL_ERROR', 'Каталог недоступен')
  if (['network-error', 'unauthorized', 'forbidden', 'server-error', 'invalid-json', 'invalid-schema', 'rate-limit'].includes(scenario)) return createHealthcheckResponse(scenario)
}
function envelope<T extends { id: string; name: string }>(items: T[], url: URL) {
  const search = (url.searchParams.get('search') ?? '').toLocaleLowerCase('ru'), page = Number(url.searchParams.get('page') ?? 1), page_size = Number(url.searchParams.get('page_size') ?? 50)
  const filtered = items.filter((item) => item.name.toLocaleLowerCase('ru').includes(search)).sort((a, b) => a.name.localeCompare(b.name, 'ru') || a.id.localeCompare(b.id))
  return { items: filtered.slice((page - 1) * page_size, page * page_size), total: filtered.length, page, page_size, meta: {} }
}
function writeError(request: Request) {
  const role = document.cookie.split(';').find((value) => value.trim().startsWith('flagman_mock_role='))?.split('=')[1]
  if (role !== 'admin' && getMockScenario() !== 'role-admin' || request.headers.get('X-CSRF-Token') !== 'mock-csrf-for-local-tests') return createMockError(403, 'FORBIDDEN', 'Изменение доступно только admin с CSRF')
  if (['network-error', 'unauthorized', 'forbidden', 'server-error', 'rate-limit'].includes(getMockScenario())) return createHealthcheckResponse(getMockScenario())
}
export const catalogProductHandlers = [
  http.get('/api/v1/products', async ({ request }) => {
    const error = await readError('products'); if (error) return error
    const url = new URL(request.url), active = url.searchParams.get('is_active') ?? 'true', category = url.searchParams.get('category_id')
    const items = getMockScenario() === 'products-empty' ? [] : catalogProducts().filter((item) => (active === 'all' || String(item.is_active) === active) && (!category || category === item.category.id))
    return Response.json(envelope(items, url), { headers: { 'X-Mock-Mode': 'true' } })
  }),
]
export const catalogHandlers = [
  http.get('/api/v1/product-categories', async ({ request }) => {
    const error = await readError('categories'); if (error) return error
    return Response.json(envelope(getMockScenario() === 'categories-empty' ? [] : catalogCategories(), new URL(request.url)))
  }),
  http.get('/api/v1/product-categories/:id', async ({ params }) => {
    const error = await readError('categories'); if (error) return error
    const category = catalogCategories().find((item) => item.id.toLowerCase() === String(params.id).toLowerCase())
    return category ? Response.json(category) : createMockError(404, 'NOT_FOUND', 'Категория не найдена')
  }),
  http.post('/api/v1/product-categories', async ({ request }) => {
    const error = writeError(request); if (error) return error
    if (getMockScenario() === 'category-save-slow') await delay(2000)
    const parsed = createCategorySchema.safeParse(await request.json())
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте название')
    if (catalogCategories().some((item) => item.name.toLocaleLowerCase('ru') === parsed.data.name.toLocaleLowerCase('ru'))) return createMockError(409, 'CONFLICT', 'Имя занято')
    const category = { id: crypto.randomUUID(), name: parsed.data.name, revision: 1 }; saveCatalogCategory(category)
    if (getMockScenario() === 'category-lost-response') return Response.error()
    return Response.json(category, { status: 201 })
  }),
  http.patch('/api/v1/product-categories/:id', async ({ request, params }) => {
    const error = writeError(request); if (error) return error
    if (getMockScenario() === 'category-save-slow') await delay(2000)
    const parsed = updateCategorySchema.safeParse(await request.json()), category = catalogCategories().find((item) => item.id === params.id)
    if (!category) return createMockError(404, 'NOT_FOUND', 'Категория не найдена')
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте название и ревизию')
    if (getMockScenario() === 'category-revision-conflict' && category.revision === 1) saveCatalogCategory({ ...category, name: `${category.name} (обновлено)`, revision: 2 })
    const current = catalogCategories().find((item) => item.id === params.id)!
    if (parsed.data.expected_revision !== current.revision) return createMockError(409, 'REVISION_CONFLICT', 'Ревизия изменилась', { current_revision: current.revision })
    if (catalogCategories().some((item) => item.id !== current.id && item.name.toLocaleLowerCase('ru') === parsed.data.name.toLocaleLowerCase('ru'))) return createMockError(409, 'CONFLICT', 'Имя занято')
    const changed = { ...current, name: parsed.data.name, revision: current.revision + 1 }; saveCatalogCategory(changed)
    if (getMockScenario() === 'category-lost-response') return Response.error()
    return Response.json(changed)
  }),
]
