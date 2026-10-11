import { delay } from 'msw'
import { http } from 'msw/http'
import type { Role } from '@/entities/session'
import { dashboardSnapshot } from './dashboard-fixtures'
import { getMockScenario } from './scenarios'
import { createHealthcheckResponse, createMockError } from './responses'

export const dashboardHandlers = [http.get('/api/v1/dashboard', async ({ request }) => {
  const scenario = getMockScenario()
  if (new URL(request.url).search) return createMockError(422, 'VALIDATION_ERROR', 'У обзора нет query-параметров')
  if (['network-error', 'forbidden', 'unauthorized', 'rate-limit', 'server-error', 'bad-gateway', 'invalid-json', 'invalid-schema'].includes(scenario)) return createHealthcheckResponse(scenario)
  const stored = typeof document === 'undefined' ? undefined : document.cookie.split(';').find((value) => value.trim().startsWith('flagman_mock_role='))?.split('=')[1]
  const role: Role = scenario === 'role-storekeeper' || stored === 'storekeeper' ? 'storekeeper' : scenario === 'role-order-operator' || stored === 'order_operator' ? 'order_operator' : 'manager'
  const snapshot = dashboardSnapshot(role)
  if (scenario === 'slow') await delay(2000)
  return Response.json(snapshot, { headers: { 'X-Mock-Mode': 'true' } })
})]
