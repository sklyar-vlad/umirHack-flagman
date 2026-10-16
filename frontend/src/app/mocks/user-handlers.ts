import { delay } from 'msw'
import { http } from 'msw/http'
import { readUserSearch, createUserSchema, updateUserSchema, resetUserPasswordSchema, type User }  from '@/entities/user'
import { mockRoles, mockUserList, getMockUser, storeMockUser, mockEmailUsed, mockLastAdmin, mockActorId } from './user-fixtures'
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
  http.get('/api/v1/users/:id', async ({ params }) => {
    const error = await readError('users'); if (error) return error
    if (getMockScenario() === 'user-not-found') return createMockError(404, 'NOT_FOUND', 'Пользователь не найден')
    const user = getMockUser(String(params.id))
    return user ? Response.json(user, { headers: { 'Cache-Control': 'no-store', 'X-Mock-Mode': 'true' } }) : createMockError(404, 'NOT_FOUND', 'Пользователь не найден')
  }),
  http.post('/api/v1/users', async ({ request }) => write(request)),
  http.patch('/api/v1/users/:id', async ({ request, params }) => write(request, String(params.id))),
  http.post('/api/v1/users/:id/password', async ({ request, params }) => write(request, String(params.id), true)),
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

async function write(request: Request, id?: string, password = false) {
  const error = await readError('users'); if (error) return error
  if (request.headers.get('X-CSRF-Token') !== 'mock-csrf-for-local-tests') return createMockError(403, 'CSRF_INVALID', 'Неверный CSRF')
  const scenario = getMockScenario()
  if (scenario === 'user-save-slow') await delay(1500)
  const input: unknown = await request.json()
  const parsed = (password ? resetUserPasswordSchema : id ? updateUserSchema : createUserSchema).safeParse(input)
  if (!parsed.success || scenario === 'user-validation') return createMockError(422, 'VALIDATION_ERROR', 'Некорректные поля', { fields: [{ path: password ? 'password' : 'email', code: 'INVALID', message: 'Проверьте значение' }] })
  const body = parsed.data
  const current = id ? getMockUser(id) : undefined
  if (id && !current) return createMockError(404, 'NOT_FOUND', 'Пользователь не найден')
  if (current && ('expected_revision' in body && body.expected_revision !== current.revision || scenario === 'user-revision-conflict')) {
    if (scenario === 'user-revision-conflict') storeMockUser({ ...current, revision: current.revision + 1, name: `${current.name} · сервер` })
    return createMockError(409, 'REVISION_CONFLICT', 'Ревизия изменена', { current_revision: current.revision + 1 })
  }
  if ('email' in body && body.email && (mockEmailUsed(body.email, current?.id) || scenario === 'user-email-conflict')) return createMockError(409, 'CONFLICT', 'Email занят')
  if (current && !password && (scenario === 'user-last-admin' || mockLastAdmin(current, 'role' in body && body.role || current.role, 'is_active' in body && body.is_active !== undefined ? body.is_active : current.is_active))) return createMockError(409, 'LAST_ADMIN_PROTECTED', 'Последний активный admin')
  const fields = Object.fromEntries(Object.entries(body).filter(([key]) => !['password', 'expected_revision'].includes(key)))
  const user = current ? { ...current, ...fields, revision: current.revision + 1 } as User : { ...fields, id: crypto.randomUUID(), revision: 1, is_active: 'is_active' in body ? body.is_active ?? true : true, created_at: new Date().toISOString(), last_login_at: null } as User
  storeMockUser(user)
  if (current?.id === mockActorId && (password || current.role !== user.role || !user.is_active)) document.cookie = 'flagman_mock_session=; Path=/; Max-Age=0'
  if (scenario === 'user-lost-response') return Response.error()
  return Response.json(user, { status: id ? 200 : 201, headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
}
