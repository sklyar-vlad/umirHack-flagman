import { delay } from 'msw'
import { http } from 'msw/http'
import { orderRequestSchema } from '@/shared/api/order-work'
import { compositionDishes, compositionCheck, changeCompositionRecipe, savedOrder, acceptOrder, rejectOrder } from './order-composition-fixtures'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'

const generalErrors = ['network-error', 'forbidden', 'unauthorized', 'rate-limit', 'server-error', 'invalid-json', 'invalid-schema']
export const orderCompositionHandlers = [
  http.get('/api/v1/dishes', async ({ request }) => {
    const scenario = getMockScenario()
    if (generalErrors.includes(scenario)) return createHealthcheckResponse(scenario)
    if (scenario === 'dishes-error') return createMockError(500, 'INTERNAL_ERROR', 'Каталог недоступен')
    if (scenario === 'dishes-slow' || scenario === 'slow') await delay(2000)
    const params = new URL(request.url).searchParams, search = params.get('search')?.toLocaleLowerCase('ru') ?? ''
    const page = Number(params.get('page') ?? 1), page_size = Number(params.get('page_size') ?? 20)
    const items = scenario === 'dishes-empty' ? [] : compositionDishes(scenario === 'order-recipe-changed', scenario === 'order-inactive').filter((item) => item.name.toLocaleLowerCase('ru').includes(search))
    return Response.json({ items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: {} })
  }),
  http.get('/api/v1/dishes/:id', async ({ params }) => {
    const scenario = getMockScenario()
    if (generalErrors.includes(scenario)) return createHealthcheckResponse(scenario)
    if (scenario === 'dish-detail-error') return createMockError(500, 'INTERNAL_ERROR', 'Состав недоступен')
    if (scenario === 'dish-detail-missing') return createMockError(404, 'NOT_FOUND', 'Блюдо не найдено')
    if (scenario === 'dish-detail-slow') await delay(1500)
    const dish = compositionDishes(scenario === 'order-recipe-changed' || scenario === 'order-final-recipe', scenario === 'order-inactive' || scenario === 'order-final-inactive').find((item) => item.id === params.id)
    return dish ? Response.json(dish) : createMockError(404, 'NOT_FOUND', 'Блюдо не найдено')
  }),
  http.post('/api/v1/orders/check', async ({ request }) => {
    const scenario = getMockScenario()
    if (scenario === 'order-check-slow' || scenario === 'slow') await delay(1500)
    if (generalErrors.includes(scenario) || scenario === 'order-check-error') return createHealthcheckResponse(scenario === 'order-check-error' ? 'server-error' : scenario)
    const parsed = orderRequestSchema.safeParse(await request.json())
    if (!parsed.success || parsed.data.items.some((line) => !compositionDishes().some((dish) => dish.id === line.dish_id))) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте корзину')
    return Response.json(compositionCheck(parsed.data, scenario === 'order-recipe-changed', scenario === 'order-inactive', scenario === 'order-check-shortage'))
  }),
  http.post('/api/v1/orders', async ({ request }) => {
    const scenario = getMockScenario()
    if (generalErrors.includes(scenario)) return createHealthcheckResponse(scenario)
    if (scenario === 'order-submit-slow') await delay(1500)
    const key = request.headers.get('Idempotency-Key') ?? '', parsed = orderRequestSchema.safeParse(await request.json())
    if (!key || !parsed.success) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте корзину и ключ')
    const body = parsed.data, prior = savedOrder(key)
    if (prior) {
      if (prior.body !== JSON.stringify(body)) return createMockError(409, 'IDEMPOTENCY_CONFLICT', 'Ключ использован с другим телом')
      return prior.order ? Response.json(prior.order, { status: 201 }) : createMockError(409, prior.reason!, 'Заказ отклонён', prior.refusal)
    }
    if (scenario === 'order-network') return Response.error()
    if (scenario === 'order-rate-limit') return createMockError(429, 'RATE_LIMIT_EXCEEDED', 'Подождите', {}, '1')
    if (scenario === 'order-validation') return createMockError(422, 'VALIDATION_ERROR', 'Проверьте порции', { fields: [{ path: 'items[0].portions', message: 'Порции отклонены сервером', code: 'invalid' }] })
    if (scenario === 'order-final-recipe' || scenario === 'order-final-inactive') changeCompositionRecipe(scenario === 'order-final-inactive')
    const check = compositionCheck(body, scenario === 'order-recipe-changed' || scenario === 'order-final-recipe', scenario === 'order-inactive' || scenario === 'order-final-inactive', scenario === 'order-shortage')
    if (!check.can_create) {
      const reason = check.issues.some((issue) => issue.code === 'recipe_changed') ? 'RECIPE_CHANGED' : check.issues.length ? 'DISH_UNAVAILABLE' : 'INSUFFICIENT_STOCK'
      const refusal = rejectOrder(key, body, check, reason)
      return createMockError(409, reason, reason === 'INSUFFICIENT_STOCK' ? 'Недостаточно продуктов' : reason === 'RECIPE_CHANGED' ? 'Рецептура изменилась' : 'Блюдо недоступно', refusal)
    }
    const order = acceptOrder(key, body, check)
    if (scenario === 'order-lost-response') return Response.error()
    return Response.json(order, { status: 201, headers: { Location: `/api/v1/orders/${order.id}`, 'X-Mock-Mode': 'true' } })
  }),
]
