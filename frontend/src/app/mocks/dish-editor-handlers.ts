import { delay } from 'msw'
import { http } from 'msw/http'
import { createDishSchema, updateDishSchema, previewDishSchema, type PreviewDish } from '@/shared/api/dish-work'
import { catalogProducts } from './catalog-fixtures'
import { dishCatalog } from './dish-catalog-fixtures'
import { editorPreview, saveEditorDish, withEditorAvailability } from './dish-editor-fixtures'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'
function access(request: Request) {
  const role = typeof document === 'undefined' ? 'admin' : document.cookie.split(';').find((value) => value.trim().startsWith('flagman_mock_role='))?.split('=')[1]
  if (role !== 'admin' && getMockScenario() !== 'role-admin' || request.headers.get('X-CSRF-Token') !== 'mock-csrf-for-local-tests') return createMockError(403, 'FORBIDDEN', 'Требуются admin и CSRF')
  if (['network-error', 'unauthorized', 'forbidden', 'server-error', 'rate-limit', 'invalid-json', 'invalid-schema'].includes(getMockScenario())) return createHealthcheckResponse(getMockScenario())
}
function recipeError(recipe: PreviewDish['recipe']) {
  const products = catalogProducts()
  for (const [index, line] of recipe.entries()) {
    const product = products.find((item) => item.id.toLowerCase() === line.product_id.toLowerCase())
    if (!product || !product.is_active || product.unit === 'pcs' && !/^\d+(?:\.0+)?$/.test(line.quantity_per_portion)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте состав', { fields: [{ path: `recipe[${index}].product_id`, code: 'invalid', message: 'Требуется активный продукт и совместимый расход' }] })
  }
}
function validation() {
  if (getMockScenario() === 'dish-duplicate') return createMockError(422, 'DUPLICATE_INGREDIENT', 'Повтор ингредиента', { fields: [{ path: 'recipe[0].product_id', code: 'duplicate', message: 'Сервер: повтор ингредиента' }] })
  if (getMockScenario() === 'dish-validation') return createMockError(422, 'VALIDATION_ERROR', 'Проверьте расход', { fields: [{ path: 'body.recipe[0].quantity_per_portion', code: 'invalid', message: 'Сервер: проверьте расход' }] })
}
export const dishEditorHandlers = [
  http.post('/api/v1/dishes/preview', async ({ request }) => {
    const denied = access(request); if (denied) return denied
    const parsed = previewDishSchema.safeParse(await request.json())
    if (getMockScenario() === 'dish-preview-slow') await delay(1500)
    if (getMockScenario() === 'dish-preview-error') return createMockError(500, 'INTERNAL_ERROR', 'Preview недоступен')
    const error = validation(); if (error) return error
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте рецепт')
    const bad = recipeError(parsed.data.recipe); if (bad) return bad
    return Response.json(editorPreview(parsed.data.recipe))
  }),
  http.post('/api/v1/dishes', async ({ request }) => {
    const denied = access(request); if (denied) return denied
    if (getMockScenario() === 'dish-save-slow') await delay(1500)
    const error = validation(); if (error) return error
    const parsed = createDishSchema.safeParse(await request.json())
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте поля')
    const bad = recipeError(parsed.data.recipe); if (bad) return bad
    if (dishCatalog().some((item) => item.name.toLocaleLowerCase('ru') === parsed.data.name.toLocaleLowerCase('ru'))) return createMockError(409, 'CONFLICT', 'Название занято')
    const dish = withEditorAvailability({ ...dishCatalog()[0], ...parsed.data, id: crypto.randomUUID(), revision: 1, recipe_version: 1, recipe: parsed.data.recipe.map((line) => { const product = catalogProducts().find((item) => item.id.toLowerCase() === line.product_id.toLowerCase())!; return { ...line, product_name: product.name, unit: product.unit } }) })
    saveEditorDish(dish)
    if (getMockScenario() === 'dish-lost-response') return Response.error()
    return Response.json(dish, { status: 201 })
  }),
  http.patch('/api/v1/dishes/:id', async ({ request, params }) => {
    const denied = access(request); if (denied) return denied
    if (getMockScenario() === 'dish-save-slow') await delay(1500)
    const error = validation(); if (error) return error
    const parsed = updateDishSchema.safeParse(await request.json()), dish = dishCatalog().find((item) => item.id === params.id)
    if (!dish) return createMockError(404, 'NOT_FOUND', 'Блюдо не найдено')
    if (!parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте поля')
    if (getMockScenario() === 'dish-revision-conflict' && dish.revision === 1) saveEditorDish({ ...dish, name: `${dish.name} (обновлено)`, revision: 2 })
    const current = dishCatalog().find((item) => item.id === params.id)!
    if (parsed.data.expected_revision !== current.revision) return createMockError(409, 'REVISION_CONFLICT', 'Блюдо изменилось', { current_revision: current.revision })
    const { expected_revision: revision, recipe, ...fields } = parsed.data
    const source = recipe ?? current.recipe
    if (recipe || fields.is_active === true) { const bad = recipeError(source); if (bad) return bad }
    if (fields.name && dishCatalog().some((item) => item.id !== current.id && item.name.toLocaleLowerCase('ru') === fields.name!.toLocaleLowerCase('ru'))) return createMockError(409, 'CONFLICT', 'Название занято')
    const changed = withEditorAvailability({ ...current, ...fields, revision: revision + 1, recipe_version: recipe ? (current.recipe_version ?? 0) + 1 : current.recipe_version, recipe: recipe ? recipe.map((line) => { const product = catalogProducts().find((item) => item.id.toLowerCase() === line.product_id.toLowerCase())!; return { ...line, product_name: product.name, unit: product.unit } }) : current.recipe })
    saveEditorDish(changed)
    if (getMockScenario() === 'dish-lost-response') return Response.error()
    return Response.json(changed)
  }),
]
