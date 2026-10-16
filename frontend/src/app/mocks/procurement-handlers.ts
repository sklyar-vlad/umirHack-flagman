import { delay } from 'msw'
import { http } from 'msw/http'
import { procurementItems } from './procurement-fixtures'
import { inventoryItems } from './inventory-fixtures'
import { getMockScenario } from './scenarios'
import { createHealthcheckResponse } from './responses'
export const procurementHandlers = [http.get('/api/v1/procurement-recommendations', async ({ request }) => {
  const scenario = getMockScenario()
  if (['network-error', 'forbidden', 'unauthorized', 'rate-limit', 'server-error', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
  const params = new URL(request.url).searchParams, search = params.get('search')?.toLocaleLowerCase('ru') ?? '', category = params.get('category_id')
  const page = Number(params.get('page') ?? 1), page_size = Number(params.get('page_size') ?? 20)
  const items = procurementItems().filter((item) => item.product_name.toLocaleLowerCase('ru').includes(search) && (!category || inventoryItems.find(({ product }) => product.id === item.product_id)?.product.category.id === category))
  const snapshot = { items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: { server_time: new Date().toISOString() } }
  if (scenario === 'slow') await delay(2000)
  return Response.json(snapshot)
})]
