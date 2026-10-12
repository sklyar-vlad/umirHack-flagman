import { delay } from 'msw'
import { http } from 'msw/http'
import { readUserSearch } from '@/entities/user'
import { mockRoles, mockUserList } from './user-fixtures'
import { createHealthcheckResponse, createMockError } from './responses'
import { getMockScenario } from './scenarios'

async function readError(scope: 'users' | 'roles') {
  const scenario = getMockScenario()
  if (scenario === 'slow' || scenario === 'users-slow' && scope === 'users' || scenario === 'roles-slow' && scope === 'roles') await delay(2000)
  const role = document.cookie.split(';').find((value) => value.trim().startsWith('flagman_mock_role='))?.split('=')[1]
  if (role !== 'admin' && scenario !== 'role-admin') return createMockError(403, 'FORBIDDEN', 'Только для администратора')
  if (scenario === 'users-error' && scope === 'users' || scenario === 'roles-error' && scope === 'roles') return createMockError(500, 'INTERNAL_ERROR', 'Список недоступен')
  if (scenario === 'roles-forbidden' && scope === 'roles') return createMockError(403, 'FORBIDDEN', 'Нет доступа к справочнику ролей')
  if (['network-error', 'unauthorized', 'forbidden', 'server-error', 'invalid-json', 'invalid-schema', 'validation-error', 'rate-limit'].includes(scenario)) return createHealthcheckResponse(scenario)
}
export const userHandlers = [
  http.get('/api/v1/roles', async () => {
    const error = await readError('roles'); if (error) return error
    return Response.json(mockRoles, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
  }),
  http.get('/api/v1/users', async ({ request }) => {
    const error = await readError('users'); if (error) return error
    const params = new URL(request.url).searchParams, values = readUserSearch(params)
    const allowed = ['search', 'role', 'is_active', 'sort', 'page', 'page_size']
    if ([...params.keys()].some((key) => !allowed.includes(key) || params.getAll(key).length !== 1) || [...params.entries()].some(([key, raw]) => key !== 'search' && String(values[key as keyof typeof values]) !== raw)) return createMockError(422, 'VALIDATION_ERROR', 'Некорректные фильтры')
    return Response.json(mockUserList(values, getMockScenario() === 'users-empty'), { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
  }),
]
