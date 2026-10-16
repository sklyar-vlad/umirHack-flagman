import { applyBatchWriteOff, removedWriteOffQuantity } from './write-off-fixtures'
import { mockSettings } from './settings-fixtures'
import { catalogCategories, catalogInventoryItems } from './catalog-fixtures'
import { consumedCompositionAmounts } from './order-composition-fixtures'
import { orderDemoInventory, orderDemoSupplies } from './order-detail-warehouse-fixtures'
import type { Batch } from '@/entities/inventory'
import { receivedSupplies } from './reception-fixtures'
import { parseQuantity } from '@/shared/lib/quantity'
import { delay } from 'msw'
import { http } from 'msw/http'
import type { InventoryFilter, InventoryResponse } from '@/entities/inventory'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario, type MockScenario } from './scenarios'
import { inventoryBatches, inventoryItems, inventoryServerTime } from './inventory-fixtures'

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
function thousandths(raw: string) { const [whole, fraction = ''] = raw.split('.'); return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0')) }
function aggregate(value: bigint) { const digits = value.toString().padStart(4, '0'); return `${digits.slice(0, -3)}.${digits.slice(-3)}` }
function currentInventoryBatches(): Batch[] {
  const consumed = consumedCompositionAmounts()
  return [...orderDemoSupplies().flatMap((supply) => supply.items.map((line) => line.batch)), ...inventoryBatches].map(applyBatchWriteOff).map((batch) => {
    const removed = consumed[batch.id] ?? 0
    const threshold = mockSettings().expiring_threshold_hours * 3600
    batch = { ...batch, status: batch.status === 'depleted' || batch.status === 'expired' ? batch.status : batch.remaining_seconds <= threshold ? 'expiring' : 'valid' }
    if (!removed) return batch
    const remaining = thousandths(batch.remaining_quantity) - parseQuantity(String(removed))!
    return { ...batch, remaining_quantity: aggregate(remaining), status: remaining === 0n ? 'depleted' : batch.status }
  })
}
export function currentMockBatches() { return [...receivedSupplies().flatMap((supply) => supply.items.map((line) => applyBatchWriteOff(line.batch))), ...currentInventoryBatches()] }
function receivedInventory() {
  const batches = receivedSupplies().flatMap((supply) => supply.items.map((line) => applyBatchWriteOff(line.batch)))
  const consumed = consumedCompositionAmounts()
  return catalogInventoryItems().map((original) => {
    const removed = inventoryBatches.filter((batch) => batch.product_id === original.product.id).reduce((sum, batch) => sum + parseQuantity(String(consumed[batch.id] ?? 0))!, 0n)
    const current = currentInventoryBatches().filter((batch) => batch.product_id === original.product.id && batch.status !== 'depleted')
    const nearestBatch = current.filter((batch) => batch.status !== 'expired').sort((a, b) => a.expires_at.localeCompare(b.expires_at))[0]
    const updated = { ...original, has_expiring: current.some((batch) => batch.status === 'expiring'), expiring_quantity: aggregate(current.filter((batch) => batch.status === 'expiring').reduce((sum, batch) => sum + thousandths(batch.remaining_quantity), 0n)) }
    let item = removed ? { ...original, has_expiring: current.some((batch) => batch.status === 'expiring'), nearest_expires_at: nearestBatch?.expires_at ?? null, remaining_seconds: nearestBatch?.remaining_seconds ?? null, batches_count: current.length, is_out_of_stock: thousandths(original.available_quantity) === removed, available_quantity: aggregate(thousandths(original.available_quantity) - removed), physical_quantity: aggregate(thousandths(original.physical_quantity) - removed), expiring_quantity: updated.expiring_quantity, deficit_quantity: original.deficit_quantity === null ? null : aggregate(thousandths(original.deficit_quantity) + removed) } : mockSettings().expiring_threshold_hours === 24 ? original : updated
    const writtenValid = removedWriteOffQuantity(original.product.id, false), writtenExpired = removedWriteOffQuantity(original.product.id, true)
    const writtenBaseValid = inventoryBatches.filter((batch) => batch.product_id === original.product.id && batch.status !== 'expired').reduce((sum, batch) => sum + parseQuantity(batch.remaining_quantity)! - parseQuantity(applyBatchWriteOff(batch).remaining_quantity)!, 0n)
    const writtenBaseExpired = inventoryBatches.filter((batch) => batch.product_id === original.product.id && batch.status === 'expired').reduce((sum, batch) => sum + parseQuantity(batch.remaining_quantity)! - parseQuantity(applyBatchWriteOff(batch).remaining_quantity)!, 0n)
    if (writtenValid || writtenExpired) {
      const available = thousandths(item.available_quantity) - writtenBaseValid, expired = thousandths(item.expired_quantity) - writtenBaseExpired
      item = { ...item, available_quantity: aggregate(available), expired_quantity: aggregate(expired), physical_quantity: aggregate(thousandths(item.physical_quantity) - writtenBaseValid - writtenBaseExpired), expiring_quantity: updated.expiring_quantity, has_expired: expired > 0n, has_expiring: current.some((batch) => batch.status === 'expiring'), batches_count: current.length, is_out_of_stock: available === 0n, nearest_expires_at: nearestBatch?.expires_at ?? null, remaining_seconds: nearestBatch?.remaining_seconds ?? null, deficit_quantity: item.target_stock === null ? null : aggregate(parseQuantity(item.target_stock)! > available ? parseQuantity(item.target_stock)! - available : 0n), status: expired > 0n ? 'expired' : current.some((batch) => batch.status === 'expiring') ? 'expiring' : available > 0n ? 'normal' : 'out_of_stock' }
    }
    const lines = batches.filter((batch) => batch.product_id === item.product.id && parseQuantity(batch.remaining_quantity)! > 0n)
    if (!lines.length) return item
    const valid = lines.filter((batch) => batch.status !== 'expired')
    const total = lines.reduce((sum, line) => sum + parseQuantity(line.remaining_quantity)!, 0n)
    const available = thousandths(item.available_quantity) + valid.reduce((sum, line) => sum + parseQuantity(line.remaining_quantity)!, 0n)
    const expired = thousandths(item.expired_quantity) + lines.filter((batch) => batch.status === 'expired').reduce((sum, line) => sum + parseQuantity(line.remaining_quantity)!, 0n)
    const expiring = thousandths(item.expiring_quantity) + lines.filter((batch) => batch.status === 'expiring').reduce((sum, line) => sum + parseQuantity(line.remaining_quantity)!, 0n)
    const deficit = item.target_stock === null ? null : parseQuantity(item.target_stock)! - available
    const nearest = [item.nearest_expires_at, ...valid.map((line) => line.expires_at)].filter((value): value is string => value !== null).sort()[0]
    return { ...item, physical_quantity: aggregate(thousandths(item.physical_quantity) + total), available_quantity: aggregate(available),
      expired_quantity: aggregate(expired), expiring_quantity: aggregate(expiring), has_expired: expired > 0n, has_expiring: expiring > 0n,
      deficit_quantity: deficit === null ? null : aggregate(deficit > 0n ? deficit : 0n), batches_count: item.batches_count + lines.length, is_out_of_stock: available === 0n,
      status: expired > 0n ? 'expired' as const : expiring > 0n ? 'expiring' as const : available > 0n ? 'normal' as const : 'out_of_stock' as const,
      nearest_expires_at: nearest ?? null, remaining_seconds: item.remaining_seconds ?? valid[0]?.remaining_seconds ?? null }

  })
}
const nonzero = (raw: string | null) => raw !== null && !/^0+(?:\.0+)?$/.test(raw)
export const inventoryHandlers = [
  http.get('/api/v1/inventory', async ({ request }) => {
    const scenario = getMockScenario(), error = await scenarioError(scenario, 'inventory')
    if (error) return error
    const url = new URL(request.url), search = (url.searchParams.get('search') ?? '').toLocaleLowerCase('ru')
    const category = url.searchParams.get('category_id'), filter = url.searchParams.get('filter') ?? 'all'
    const area = scenario === 'inventory-empty' ? [] : [...orderDemoInventory(), ...receivedInventory()].filter((item) => item.product.name.toLocaleLowerCase('ru').includes(search) && (!category || item.product.category.id === category))
    const matches = (item: typeof inventoryItems[number], key: InventoryFilter) => key === 'all' || key === 'expiring' && item.has_expiring || key === 'expired' && item.has_expired || key === 'out_of_stock' && item.is_out_of_stock || key === 'deficit' && nonzero(item.deficit_quantity)
    const items = area.filter((item) => matches(item, filter as InventoryFilter)).sort((a, b) => {
      if (url.searchParams.get('sort') === 'nearest_expires_at') {
        const aa = a.nearest_expires_at, bb = b.nearest_expires_at
        if (aa !== bb) return aa === null ? 1 : bb === null ? -1 : aa.localeCompare(bb)
      }
      return a.product.name.localeCompare(b.product.name, 'ru') || a.product.id.localeCompare(b.product.id)
    })
    const counts = Object.fromEntries(['all', 'expiring', 'expired', 'out_of_stock', 'deficit'].map((key) => [key, area.filter((item) => matches(item, key as InventoryFilter)).length])) as InventoryResponse['counts']
    const attention = currentInventoryBatches().filter((batch) => area.some((item) => item.product.id === batch.product_id) && (batch.status === 'expired' || batch.status === 'expiring')).sort((a, b) => a.expires_at.localeCompare(b.expires_at) || a.id.localeCompare(b.id)).map((batch) => ({
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
    return response(pageEnvelope(catalogCategories(), new URL(request.url), {}))
  }),
  http.get('/api/v1/products/:id/batches', async ({ request, params }) => {
    const scenario = getMockScenario(), error = await scenarioError(scenario, 'batches')
    if (error) return error
    const url = new URL(request.url), status = url.searchParams.get('status')
    const items = scenario === 'batches-empty' ? [] : currentMockBatches().filter((batch) => batch.product_id === params.id && (status === 'all' || (!status ? batch.status !== 'depleted' : batch.status === status))).sort((a, b) => (url.searchParams.get('sort') === '-created_at' ? b.created_at.localeCompare(a.created_at) : a.expires_at.localeCompare(b.expires_at)) || a.id.localeCompare(b.id))
    return response(pageEnvelope(items, url, { server_time: inventoryServerTime }))
  }),
]
