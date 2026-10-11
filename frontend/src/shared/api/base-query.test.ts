import { afterEach, describe, expect, it, vi } from 'vitest'
import type { BaseQueryApi } from '@reduxjs/toolkit/query'
import { baseQuery } from './base-query'
import type { ApiTransport } from './transport'
function api(extra: ApiTransport): BaseQueryApi {
  return { extra, signal: new AbortController().signal, abort: vi.fn(), dispatch: vi.fn(), getState: () => ({}), endpoint: 'test', type: 'query' }
}
afterEach(() => vi.unstubAllGlobals())
describe('business request session barrier', () => {
  it('never sends a business request when restoration denies access', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>()
    vi.stubGlobal('fetch', fetch)
    const result = await baseQuery('http://localhost/api/v1/inventory', api({ getCsrfToken: () => null, waitForSession: async () => false }), {})
    expect(result.error).toMatchObject({ status: 401, code: 'SESSION_REQUIRED' })
    expect(fetch).not.toHaveBeenCalled()
  })
  it('waits for restoration, attaches CSRF on writes and makes one request', async () => {
    let release!: (value: boolean) => void
    const wait = new Promise<boolean>((resolve) => { release = resolve })
    const fetch = vi.fn<typeof globalThis.fetch>(async () => Response.json({ ok: true }))
    vi.stubGlobal('fetch', fetch)
    const result = baseQuery({ url: 'http://localhost/api/v1/supplies', method: 'POST', body: {} }, api({ getCsrfToken: () => 'test-csrf', waitForSession: () => wait }), {})
    expect(fetch).not.toHaveBeenCalled()
    release(true)
    expect((await result).data).toEqual({ ok: true })
    expect(fetch).toHaveBeenCalledTimes(1)
    const request = fetch.mock.calls[0][0] as Request
    expect(request.headers.get('X-CSRF-Token')).toBe('test-csrf')
    expect(request.credentials).toBe('include')
  })
  it('centralizes 401 without automatically retrying a mutation', async () => {
    const onUnauthorized = vi.fn()
    const fetch = vi.fn<typeof globalThis.fetch>(async () => Response.json({}, { status: 401 }))
    vi.stubGlobal('fetch', fetch)
    const result = await baseQuery({ url: 'http://localhost/api/v1/orders', method: 'POST', body: {} }, api({ getCsrfToken: () => null, waitForSession: async () => true, onUnauthorized }), {})
    expect(result.error?.status).toBe(401)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('rechecks access after the asynchronous barrier when logout occurs between microtasks', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const result = await baseQuery('http://localhost/api/v1/inventory', api({ getCsrfToken: () => null, waitForSession: async () => true, hasSessionAccess: () => false }), {})
    expect(result.error?.code).toBe('SESSION_CHANGED')
    expect(fetch).not.toHaveBeenCalled()
  })
  it('drops late responses from a previous session', async () => {
    let version = 1
    const fetch = vi.fn<typeof globalThis.fetch>(async () => { version++; return Response.json({ private: 'old-user' }) })
    vi.stubGlobal('fetch', fetch)
    const result = await baseQuery('http://localhost/api/v1/inventory', api({ getCsrfToken: () => null, waitForSession: async () => true, getSessionVersion: () => version }), {})
    expect(result.data).toBeUndefined()
    expect(result.error?.code).toBe('SESSION_CHANGED')
  })
})
