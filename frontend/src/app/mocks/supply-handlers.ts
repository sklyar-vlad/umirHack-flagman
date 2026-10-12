import { applyBatchWriteOff } from './write-off-fixtures'
import type { SupplyDetail } from '@/entities/supply'
import { receivedSupplies } from './reception-fixtures'
import { orderDemoSupplies } from './order-detail-warehouse-fixtures'
import { z } from 'zod'
import { supplyDetailFixture } from './supply-detail-fixtures'
import { delay } from 'msw'
import { http } from 'msw/http'
import { compareSupplyInstants } from '@/entities/supply'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'
import { newlyReceivedSupply, supplyFixtures } from './supply-fixtures'

function withCurrentWriteOffStock(supply: SupplyDetail): SupplyDetail { return { ...supply, items: supply.items.map((line) => ({ ...line, batch: applyBatchWriteOff(line.batch) })) } }

export const supplyHandlers = [
  http.get('/api/v1/supplies/:id', async ({ params }) => {
    const scenario = getMockScenario()
    if (scenario === 'slow') await delay(2000)
    if (['network-error', 'unauthorized', 'forbidden', 'conflict', 'validation-error', 'rate-limit', 'server-error', 'bad-gateway', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
    if (!z.uuid().safeParse(params.id).success) return createMockError(422, 'VALIDATION_ERROR', 'Некорректный идентификатор')
    const received = [...receivedSupplies(), ...orderDemoSupplies()].find((supply) => supply.id === params.id)
    if (received) return Response.json(withCurrentWriteOffStock(received))
    const summary = [newlyReceivedSupply, ...supplyFixtures].find((item) => item.id === String(params.id).toLowerCase())
    if (!summary || scenario === 'supply-not-found') return createMockError(404, 'NOT_FOUND', 'Поставка не найдена')
    return Response.json(withCurrentWriteOffStock(supplyDetailFixture(summary)), { headers: { 'X-Mock-Mode': 'true' } })
  }),
  http.get('/api/v1/supplies', async ({ request }) => {
    const scenario = getMockScenario()
    if (scenario === 'slow') await delay(2000)
    if (['network-error', 'unauthorized', 'forbidden', 'conflict', 'validation-error', 'rate-limit', 'server-error', 'bad-gateway', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
    const url = new URL(request.url), search = (url.searchParams.get('search') ?? '').toLocaleLowerCase('ru')
    const from = url.searchParams.get('from'), to = url.searchParams.get('to')
    const source = scenario === 'supplies-empty' ? [] : scenario === 'supplies-received' ? [newlyReceivedSupply, ...supplyFixtures] : supplyFixtures
    const items = [...receivedSupplies(), ...source].filter((item) => item.number.toLocaleLowerCase('ru').includes(search)
      && (!from || compareSupplyInstants(item.created_at, from) >= 0) && (!to || compareSupplyInstants(item.created_at, to) < 0))
      .sort((a, b) => compareSupplyInstants(b.created_at, a.created_at) || b.id.localeCompare(a.id))
    const page = Number(url.searchParams.get('page') ?? 1), page_size = Number(url.searchParams.get('page_size') ?? 50)
    return Response.json({ items: items.slice((page - 1) * page_size, page * page_size), total: items.length, page, page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true' } })
  }),
]
