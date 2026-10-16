import { delay } from 'msw'
import { http } from 'msw/http'
import { createMockError } from './responses'
import { getMockSessionUser } from './user-fixtures'
import { getMockScenario } from './scenarios'
import { mockSettings } from './settings-fixtures'

const cookieName = 'flagman_mock_session'
const mockCsrf = 'mock-csrf-for-local-tests'
const fixture = {
  user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тестовый сотрудник', email: 'employee@example.test', role: 'manager', is_active: true, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: null },
  settings: { restaurant_name: 'Тестовый ресторан', timezone: 'Europe/Moscow', expiring_threshold_hours: 24, auto_writeoff_expired: false, revision: 1 }, csrf_token: mockCsrf, session_expires_at: '2099-01-01T00:00:00Z',
}
function sessionCookie(value: string, expired = false) {
  return `${cookieName}=${value}; Path=/; SameSite=Lax; ${expired ? 'Max-Age=0' : 'Max-Age=3600'}`
}
function roleFixture() {
  const scenario = getMockScenario()
  const stored = document.cookie.split(';').find((value) => value.trim().startsWith('flagman_mock_role='))?.split('=')[1]
  const selected = scenario === 'role-admin' ? 'admin' : scenario === 'role-storekeeper' ? 'storekeeper' : scenario === 'role-order-operator' ? 'order_operator' : null
  const role = selected ?? (['admin', 'storekeeper', 'order_operator'].includes(stored ?? '') ? stored : 'manager')
  return { ...fixture, settings: mockSettings(), user: getMockSessionUser() ?? { ...fixture.user, role } }
}
function hasSession() {
  return document.cookie.split(';').some((value) => value.trim() === `${cookieName}=active`)
}
export const authHandlers = [
  http.get('/api/v1/auth/me', async () => {
    const scenario = getMockScenario()
    if (scenario === 'slow') await delay(2_000)
    if (scenario === 'network-error') return Response.error()
    if (scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка сервера')
    if (scenario === 'invalid-schema') return Response.json({ user: {} })
    if (scenario === 'disabled-user') return createMockError(403, 'USER_DISABLED', 'Учётная запись отключена')
    if (!hasSession() || scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
    return Response.json(roleFixture(), { headers: { 'X-Mock-Mode': 'true' } })
  }),
  http.post('/api/v1/auth/login', async ({ request }) => {
    const scenario = getMockScenario()
    if (scenario === 'slow') await delay(2_000)
    if (scenario === 'network-error') return Response.error()
    if (scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка сервера')
    if (scenario === 'rate-limit') return createMockError(429, 'RATE_LIMIT_EXCEEDED', 'Слишком много попыток', {}, '2')
    if (scenario === 'disabled-user') return createMockError(403, 'USER_DISABLED', 'Учётная запись отключена')
    const body = await request.json() as { email?: string; password?: string }
    if (scenario === 'invalid-password' || body.email !== fixture.user.email || body.password !== 'local-test-only') return createMockError(401, 'UNAUTHORIZED', 'Неверные данные')
    if (scenario === 'invalid-schema') return Response.json({ user: {} })
    document.cookie = sessionCookie('active')
    const { user, csrf_token, session_expires_at } = roleFixture()
    document.cookie = `flagman_mock_role=${user.role}; Path=/; SameSite=Lax; Max-Age=3600`
    return Response.json({ user, csrf_token, session_expires_at }, { headers: { 'X-Mock-Mode': 'true' } })
  }),
  http.post('/api/v1/auth/logout', async ({ request }) => {
    if (getMockScenario() === 'network-error') return Response.error()
    if (request.headers.get('X-CSRF-Token') !== mockCsrf) return createMockError(403, 'CSRF_INVALID', 'Неверный CSRF')
    document.cookie = sessionCookie('', true)
    document.cookie = 'flagman_mock_role=; Path=/; SameSite=Lax; Max-Age=0'
    return new Response(null, { status: 204, headers: { 'X-Mock-Mode': 'true' } })
  }),
]
