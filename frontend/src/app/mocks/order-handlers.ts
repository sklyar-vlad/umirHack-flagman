import { delay } from 'msw'
import { http } from 'msw/http'
import { compareHistoryInstants } from '@/shared/lib/history'
import { composedOrders, composedAttempts } from './order-composition-fixtures'
import { attemptFixtures, orderFixtures } from './order-fixtures'
import { createHealthcheckResponse } from './responses'
import { getMockScenario } from './scenarios'

export const orderHandlers = (['orders', 'order-attempts'] as const).map((resource) => http.get(`/api/v1/${resource}`, async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'slow') await delay(2000)
  if (['network-error', 'unauthorized', 'forbidden', 'conflict', 'validation-error', 'rate-limit', 'server-error', 'bad-gateway', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
  const url = new URL(request.url), params = url.searchParams
  const search = (params.get('search') ?? '').toLocaleLowerCase('ru'), from = params.get('from'), to = params.get('to'), reason = params.get('reason_code')
  const source = scenario === 'orders-empty' ? [] : resource === 'orders' ? [...composedOrders(), ...orderFixtures] : [...composedAttempts(), ...attemptFixtures]
  const items = source.filter((item) => [('number' in item ? item.number : item.id), ...item.items.map((dish) => dish.dish_name)].some((text) => text.toLocaleLowerCase('ru').includes(search))
    && (!from || compareHistoryInstants(item.created_at, from) >= 0) && (!to || compareHistoryInstants(item.created_at, to) < 0)
    && (!reason || 'reason_code' in item && item.reason_code === reason))
    .sort((a, b) => compareHistoryInstants(b.created_at, a.created_at) || b.id.localeCompare(a.id))
  const page = Number(params.get('page') ?? 1), page_size = Number(params.get('page_size') ?? 50)
  return Response.json({ items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true' } })
}))
