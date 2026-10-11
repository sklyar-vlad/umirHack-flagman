import { describe, expect, it } from 'vitest'
import { safeReturnUrl, loginUrl } from './return-url'
import { loginSchema, retryAfterDeadline } from './login-schema'

describe('safe login navigation and validation', () => {
  it.each(['https://evil.test', '//evil.test', '/\\evil.test', '/%2f%2fevil.test', '/login', '/login/', '/LOGIN?returnTo=/login', '/x/../login', '/ x', undefined])('rejects unsafe or looping return URL %s', (value) => {
    expect(safeReturnUrl(value)).toBe('/')
  })
  it('retains an internal path while dropping query strings and hashes', () => {
    expect(safeReturnUrl('/inventory?password=secret#token')).toBe('/inventory')
    expect(loginUrl('/inventory?q=test')).toBe('/login?returnTo=%2Finventory')
  })
  it('validates email and required password without inventing password policy', () => {
    expect(loginSchema.safeParse({ email: 'bad', password: '' }).success).toBe(false)
    expect(loginSchema.parse({ email: ' employee@example.test ', password: ' a ' })).toEqual({ email: 'employee@example.test', password: ' a ' })
  })
  it('honors both seconds and HTTP-date Retry-After', () => {
    expect(retryAfterDeadline('5', 1000)).toBe(6000)
    expect(retryAfterDeadline('Thu, 01 Jan 1970 00:00:10 GMT', 1000)).toBe(10000)
    expect(retryAfterDeadline('Thu, 01 Jan 1970 00:00:00 GMT', 1000)).toBe(1000)
    expect(retryAfterDeadline('bad', 1000)).toBe(1000)
  })
})
