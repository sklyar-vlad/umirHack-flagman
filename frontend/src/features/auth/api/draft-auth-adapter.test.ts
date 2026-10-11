import { afterEach, describe, expect, it, vi } from 'vitest'
import { draftAuthAdapter } from './draft-auth-adapter'
const fixture = { user: { id: '11111111-1111-4111-8111-111111111111', name: 'Тест', email: 'employee@example.test', role: 'manager', is_active: true, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: null }, settings: { timezone: 'Europe/Moscow', restaurant_name: 'Тест', expiring_threshold_hours: 24, auto_writeoff_expired: false, revision: 1 }, csrf_token: 'test-csrf', session_expires_at: '2099-01-01T00:00:00Z' }
afterEach(() => vi.unstubAllGlobals())
describe('isolated draft wire adapter', () => {
  it('sends credentials only in POST body with cookies; decodes user/settings/CSRF', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input) => Response.json(input === '/api/v1/auth/login' ? { user: fixture.user, csrf_token: fixture.csrf_token, session_expires_at: fixture.session_expires_at } : fixture))
    vi.stubGlobal('fetch', fetch)
    expect(await draftAuthAdapter.login({ email: 'employee@example.test', password: 'test-secret' }, new AbortController().signal)).toEqual({ user: { id: fixture.user.id, name: fixture.user.name, email: fixture.user.email, role: fixture.user.role }, settings: { timezone: fixture.settings.timezone, restaurantName: 'Тест', expiringThresholdHours: 24, autoWriteoffExpired: false, revision: 1 }, csrfToken: fixture.csrf_token, expiresAt: fixture.session_expires_at })
    expect(fetch).toHaveBeenCalledWith('/api/v1/auth/login', expect.objectContaining({ method: 'POST', credentials: 'include', body: JSON.stringify({ email: 'employee@example.test', password: 'test-secret' }) }))
    expect(fetch.mock.calls[0][0]).not.toContain('test-secret')
  })
  it('rejects malformed and incomplete snapshots, including unknown timezone/role', async () => {
    for (const body of [{}, { ...fixture, csrf_token: '' }, { ...fixture, settings: { timezone: 'unknown' } }, { ...fixture, user: { ...fixture.user, role: 'superuser' } }]) {
      vi.stubGlobal('fetch', vi.fn(async () => Response.json(body)))
      await expect(draftAuthAdapter.me(new AbortController().signal)).rejects.toMatchObject({ status: 'INVALID_RESPONSE' })
    }
  })
  it.each([401, 403, 429, 500])('returns safe auth error for %s without echoed request secrets', async (status) => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: { code: 'DENIED', message: 'test-secret', details: { password: 'test-secret' } }, request_id: 'test-request' }, { status, headers: { 'Retry-After': '5' } })))
    try { await draftAuthAdapter.login({ email: 'employee@example.test', password: 'test-secret' }, new AbortController().signal); throw new Error('Expected failure') }
    catch (error) { expect(error).toMatchObject({ status, retry_after: '5', details: {} }); expect(JSON.stringify(error)).not.toContain('test-secret') }
  })
  it('requires me/settings after login and rejects a mismatching user', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input) => Response.json(input === '/api/v1/auth/login' ? { user: fixture.user, csrf_token: fixture.csrf_token, session_expires_at: fixture.session_expires_at } : { ...fixture, user: { ...fixture.user, id: '22222222-2222-4222-8222-222222222222' } }))
    vi.stubGlobal('fetch', fetch)
    await expect(draftAuthAdapter.login({ email: fixture.user.email, password: 'local-test' }, new AbortController().signal)).rejects.toMatchObject({ status: 'INVALID_RESPONSE' })
    expect(fetch.mock.calls.map(([input]) => input)).toEqual(['/api/v1/auth/login', '/api/v1/auth/me'])
  })
  it('rejects disabled users and expired snapshots even with HTTP 200', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ ...fixture, user: { ...fixture.user, is_active: false } })))
    await expect(draftAuthAdapter.me(new AbortController().signal)).rejects.toMatchObject({ status: 403 })
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ ...fixture, session_expires_at: '2000-01-01T00:00:00Z' })))
    await expect(draftAuthAdapter.me(new AbortController().signal)).rejects.toMatchObject({ status: 401 })
  })
  it('logout carries CSRF, supports 204 and never resends credentials', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async () => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetch)
    await draftAuthAdapter.logout('test-csrf', new AbortController().signal)
    expect(fetch).toHaveBeenCalledWith('/api/v1/auth/logout', expect.objectContaining({ headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf' }) }))
  })
})
