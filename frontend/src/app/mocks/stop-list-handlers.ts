import { delay } from 'msw'
import { http } from 'msw/http'
import { stopListDishes } from './stop-list-fixtures'
import { getMockScenario } from './scenarios'
import { createHealthcheckResponse } from './responses'

export const stopListHandlers = [http.get('/api/v1/stop-list', async ({ request }) => {
  const scenario = getMockScenario()
  if (['network-error', 'forbidden', 'unauthorized', 'rate-limit', 'server-error', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
  const params = new URL(request.url).searchParams, search = params.get('search')?.toLocaleLowerCase('ru') ?? ''
  const page = Number(params.get('page') ?? 1), page_size = Number(params.get('page_size') ?? 20)
  const items = stopListDishes().filter((dish) => dish.name.toLocaleLowerCase('ru').includes(search))
  const snapshot = { items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: { server_time: new Date().toISOString() } }
  if (scenario === 'slow') await delay(2000)
  return Response.json(snapshot)
})]
