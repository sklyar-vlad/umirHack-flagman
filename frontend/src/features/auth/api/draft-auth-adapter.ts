import { z } from 'zod'
import type { SessionSnapshot } from '@/entities/session'
import { normalizeApiError, type ApiError } from '@/shared/api'

const draftUserSchema = z.object({
  id: z.string().uuid(), name: z.string().min(1), email: z.email(), role: z.enum(['admin', 'manager', 'storekeeper', 'order_operator']),
  is_active: z.boolean(), revision: z.number().int().positive(), created_at: z.iso.datetime({ offset: true }), last_login_at: z.iso.datetime({ offset: true }).nullable(),
})
const draftLoginSchema = z.object({ user: draftUserSchema, csrf_token: z.string().min(1), session_expires_at: z.iso.datetime({ offset: true }) })
const draftSessionSchema = draftLoginSchema.extend({
  settings: z.object({
    restaurant_name: z.string().min(1),
    timezone: z.string().refine((value) => { try { new Intl.DateTimeFormat('ru', { timeZone: value }); return true } catch { return false } }),
    expiring_threshold_hours: z.number().int().min(1).max(720), auto_writeoff_expired: z.boolean(), revision: z.number().int().positive(),
  }),
})
export interface LoginCredentials { email: string; password: string }
export interface AuthAdapter {
  me: (signal: AbortSignal) => Promise<SessionSnapshot>
  login: (credentials: LoginCredentials, signal: AbortSignal) => Promise<SessionSnapshot>
  logout: (csrfToken: string | null, signal: AbortSignal) => Promise<void>
}
const invalidResponse: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Сервер вернул некорректные данные сессии.', details: {} }

async function request(path: string, signal: AbortSignal, init?: RequestInit): Promise<unknown> {
  let response: Response
  try {
    response = await fetch(`/api/v1/auth/${path}`, {
      ...init, credentials: 'include', signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
      headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
    })
  } catch {
    throw { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Не удалось связаться с сервером. Повторите попытку.', details: {} } satisfies ApiError
  }
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    const error = normalizeApiError({ status: response.status, data: body }, { request: new Request(response.url || 'http://localhost'), response })
    throw { ...error, details: {}, message: authErrorMessage(error) }
  }
  return body
}
function decode(value: unknown): SessionSnapshot {
  const result = draftSessionSchema.safeParse(value)
  if (!result.success) throw invalidResponse
  const data = result.data
  if (!data.user.is_active) throw { status: 403, code: 'USER_DISABLED', message: 'Вход запрещён.', details: {} } satisfies ApiError
  if (Date.parse(data.session_expires_at) <= Date.now()) throw { status: 401, code: 'UNAUTHORIZED', message: 'Сессия истекла.', details: {} } satisfies ApiError
  return {
    user: { id: data.user.id, name: data.user.name, email: data.user.email, role: data.user.role },
    settings: { timezone: data.settings.timezone, restaurantName: data.settings.restaurant_name, expiringThresholdHours: data.settings.expiring_threshold_hours, autoWriteoffExpired: data.settings.auto_writeoff_expired, revision: data.settings.revision },
    csrfToken: data.csrf_token, expiresAt: data.session_expires_at,
  }
}
export function authErrorMessage(error: ApiError): string {
  if (error.status === 401) return 'Неверный email или пароль.'
  if (error.status === 403) return 'Учётная запись отключена или вход запрещён. Обратитесь к администратору.'
  if (error.status === 429) return 'Слишком много попыток входа. Подождите перед повтором.'
  if (error.status === 'INVALID_RESPONSE') return invalidResponse.message
  return 'Не удалось войти. Проверьте соединение и повторите попытку.'
}
export const draftAuthAdapter: AuthAdapter = {
  me: async (signal) => decode(await request('me', signal)),
  login: async (credentials, signal) => {
    const result = draftLoginSchema.safeParse(await request('login', signal, { method: 'POST', body: JSON.stringify(credentials) }))
    if (!result.success) throw invalidResponse
    const snapshot = decode(await request('me', signal))
    if (snapshot.user.id !== result.data.user.id) throw invalidResponse
    return snapshot
  },
  logout: async (csrfToken, signal) => { await request('logout', signal, { method: 'POST', headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {} }) },
}
