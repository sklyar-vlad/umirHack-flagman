import { delay } from 'msw'
import { http } from 'msw/http'
import { hotOfferItems } from './hot-offer-fixtures'
import { getMockScenario } from './scenarios'
import { createHealthcheckResponse } from './responses'
export const hotOfferHandlers = [http.get('/api/v1/hot-offers', async ({ request }) => {
  const scenario = getMockScenario()
  if (['network-error', 'forbidden', 'unauthorized', 'rate-limit', 'server-error', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
  const params = new URL(request.url).searchParams, search = params.get('search')?.toLocaleLowerCase('ru') ?? ''
  const page = Number(params.get('page') ?? 1), page_size = Number(params.get('page_size') ?? 50)
  const items = hotOfferItems().filter((item) => item.dish.name.toLocaleLowerCase('ru').includes(search)).map((item, i) => ({ ...item, rank: i + 1 }))
  const snapshot = { items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: { server_time: new Date().toISOString() } }
  if (scenario === 'slow') await delay(2000)
  return Response.json(snapshot)
})]
