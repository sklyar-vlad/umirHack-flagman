import { delay } from 'msw'
import { http } from 'msw/http'
import type { InventoryFilter, InventoryResponse } from '@/entities/inventory'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario, type MockScenario } from './scenarios'
import { inventoryBatches, inventoryCategories, inventoryItems, inventoryServerTime } from './inventory-fixtures'

async function scenarioError(scenario: MockScenario, scope: 'inventory' | 'batches' | 'categories') {
  if (scenario === 'slow' || scope === 'batches' && scenario === 'batches-slow') await delay(2000)
  if (scope === 'batches' && scenario === 'batches-error' || scope === 'categories' && scenario === 'categories-error') return createMockError(500, 'INTERNAL_ERROR', 'Тестовая ошибка панели')
  if (['network-error', 'unauthorized', 'forbidden', 'conflict', 'validation-error', 'rate-limit', 'server-error', 'bad-gateway', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
}
function pageEnvelope<T>(items: T[], url: URL, meta: object) {
  const page = Number(url.searchParams.get('page') ?? 1), page_size = Number(url.searchParams.get('page_size') ?? 50)
  return { items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta }
}
function response(data: unknown) { return Response.json(data, { headers: { 'X-Mock-Mode': 'true' } }) }
const nonzero = (raw: string | null) => raw !== null && !/^0+(?:\.0+)?$/.test(raw)
export const inventoryHandlers = [
  http.get('/api/v1/inventory', async ({ request }) => {
    const scenario = getMockScenario(), error = await scenarioError(scenario, 'inventory')
    if (error) return error
    const url = new URL(request.url), search = (url.searchParams.get('search') ?? '').toLocaleLowerCase('ru')
    const category = url.searchParams.get('category_id'), filter = url.searchParams.get('filter') ?? 'all'
    const area = scenario === 'inventory-empty' ? [] : inventoryItems.filter((item) => item.product.name.toLocaleLowerCase('ru').includes(search) && (!category || item.product.category.id === category))
    const matches = (item: typeof inventoryItems[number], key: InventoryFilter) => key === 'all' || key === 'expiring' && item.has_expiring || key === 'expired' && item.has_expired || key === 'out_of_stock' && item.is_out_of_stock || key === 'deficit' && nonzero(item.deficit_quantity)
    const items = area.filter((item) => matches(item, filter as InventoryFilter)).sort((a, b) => {
      if (url.searchParams.get('sort') === 'nearest_expires_at') {
        const aa = a.nearest_expires_at, bb = b.nearest_expires_at
        if (aa !== bb) return aa === null ? 1 : bb === null ? -1 : aa.localeCompare(bb)
      }
      return a.product.name.localeCompare(b.product.name, 'ru') || a.product.id.localeCompare(b.product.id)
    })
    const counts = Object.fromEntries(['all', 'expiring', 'expired', 'out_of_stock', 'deficit'].map((key) => [key, area.filter((item) => matches(item, key as InventoryFilter)).length])) as InventoryResponse['counts']
    const attention = inventoryBatches.filter((batch) => area.some((item) => item.product.id === batch.product_id) && (batch.status === 'expired' || batch.status === 'expiring')).sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id)).map((batch) => ({
      batch_id: batch.id, product_id: batch.product_id, product_name: area.find((item) => item.product.id === batch.product_id)!.product.name,
      supply_id: batch.supply.id, supply_number: batch.supply.number, line_number: batch.line_number,
      remaining_quantity: batch.remaining_quantity, unit: batch.unit, status: batch.status as 'expired' | 'expiring', expires_at: batch.expires_at, remaining_seconds: batch.remaining_seconds,
    }))
    const body: InventoryResponse = { ...pageEnvelope(items, url, { server_time: inventoryServerTime }), meta: { server_time: inventoryServerTime }, counts, attention_batches: attention.slice(0, 20), attention_total: attention.length }
    return response(body)
  }),
  http.get('/api/v1/product-categories', async ({ request }) => {
    const scenario = getMockScenario(), error = await scenarioError(scenario, 'categories')
    if (error) return error
    return response(pageEnvelope(inventoryCategories, new URL(request.url), {}))
  }),
  http.get('/api/v1/products/:id/batches', async ({ request, params }) => {
    const scenario = getMockScenario(), error = await scenarioError(scenario, 'batches')
    if (error) return error
    const url = new URL(request.url), status = url.searchParams.get('status')
    const items = scenario === 'batches-empty' ? [] : inventoryBatches.filter((batch) => batch.product_id === params.id && (status === 'all' || (!status ? batch.status !== 'depleted' : batch.status === status))).sort((a, b) => (url.searchParams.get('sort') === '-created_at' ? b.created_at.localeCompare(a.created_at) : a.expires_at.localeCompare(b.expires_at)) || a.id.localeCompare(b.id))
    return response(pageEnvelope(items, url, { server_time: inventoryServerTime }))
  }),
]
