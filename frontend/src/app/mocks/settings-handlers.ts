import { delay } from 'msw'
import { http } from 'msw/http'
import { updateSettingsSchema } from '@/shared/api/settings-contracts'
import { getMockScenario } from './scenarios'
import { createMockError } from './responses'
import { mockSettings, saveMockSettings } from './settings-fixtures'

function access(request: Request, write = false) {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (scenario === 'forbidden') return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (write && request.headers.get('X-CSRF-Token') !== 'mock-csrf-for-local-tests') return createMockError(403, 'CSRF_INVALID', 'Неверный CSRF')
}
export const settingsHandlers = [
  http.get('/api/v1/settings', async ({ request }) => {
    const scenario = getMockScenario(), denied = access(request)
    if (denied) return denied
    if (scenario === 'settings-read-slow') await delay(2000)
    if (scenario === 'settings-read-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
    if (scenario === 'invalid-schema') return Response.json({ timezone: 'Local' })
    return Response.json(mockSettings(), { headers: { 'X-Mock-Mode': 'true' } })
  }),
  http.patch('/api/v1/settings', async ({ request }) => {
    const scenario = getMockScenario(), denied = access(request, true)
    if (denied) return denied
    if (scenario === 'settings-save-slow') await delay(2000)
    const parsed = updateSettingsSchema.safeParse(await request.json())
    if (!parsed.success || scenario === 'settings-validation') return createMockError(422, 'VALIDATION_ERROR', 'Проверьте поля', { fields: [{ path: 'expiring_threshold_hours', code: 'invalid', message: 'Недопустимое значение' }] })
    const body = parsed.data, current = mockSettings()
    if (body.auto_writeoff_expired && scenario === 'settings-feature-unavailable') return createMockError(503, 'FEATURE_UNAVAILABLE', 'Worker не готов')
    if (scenario === 'settings-revision-conflict' && body.expected_revision === current.revision) saveMockSettings({ expected_revision: current.revision, restaurant_name: `${current.restaurant_name} · сервер` })
    if (body.expected_revision !== mockSettings().revision) return createMockError(409, 'REVISION_CONFLICT', 'Настройки изменены', { current_revision: mockSettings().revision })
    const next = saveMockSettings(body)
    if (scenario === 'settings-lost-response') return Response.error()
    return Response.json(next, { headers: { 'X-Mock-Mode': 'true' } })
  }),
]
