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
  it('retains menu filters across login without carrying credentials', () => {
    expect(safeReturnUrl('/menu?search=Soup&availability=available&page=2&token=secret#password')).toBe('/menu?search=Soup&availability=available&page=2')
  })
  it('retains only the published inventory filters across login', () => {
    expect(safeReturnUrl('/inventory?search=%D0%9C%D0%BE%D0%BB%D0%BE%D0%BA%D0%BE&filter=expired&page=2&page_size=20&password=secret#token')).toBe('/inventory?search=%D0%9C%D0%BE%D0%BB%D0%BE%D0%BA%D0%BE&filter=expired&page=2&page_size=20')
    expect(safeReturnUrl('/inventory?sort=-name&category_id=invalid&page=0&filter=invalid')).toBe('/inventory')
    expect(safeReturnUrl('/orders?search=test&page=2')).toBe('/orders')
    expect(loginUrl('/inventory?filter=deficit')).toBe('/login?returnTo=%2Finventory%3Ffilter%3Ddeficit')
  })
  it('validates email and required password without inventing password policy', () => {
    expect(loginSchema.safeParse({ email: 'bad', password: '' }).success).toBe(false)
    expect(loginSchema.parse({ email: ' employee@example.test ', password: ' a ' })).toEqual({ email: 'employee@example.test', password: ' a ' })
  })
  it('allowlists supply list filters and nested return links without credentials', () => {
    expect(safeReturnUrl('/supplies?search=104&page=2&token=secret#hash')).toBe('/supplies?search=104&page=2')
    expect(safeReturnUrl('/supplies/new?returnTo=https%3A%2F%2Fevil.test&password=secret')).toBe('/supplies/new?returnTo=%2Fsupplies')
    expect(safeReturnUrl('/supplies/new?returnTo=%2Fsupplies%3Fsearch%3D104%26page_size%3D20')).toBe('/supplies/new?returnTo=%2Fsupplies%3Fsearch%3D104%26page_size%3D20')
  })
  it('honors both seconds and HTTP-date Retry-After', () => {
    expect(retryAfterDeadline('5', 1000)).toBe(6000)
    expect(retryAfterDeadline('Thu, 01 Jan 1970 00:00:10 GMT', 1000)).toBe(10000)
    expect(retryAfterDeadline('Thu, 01 Jan 1970 00:00:00 GMT', 1000)).toBe(1000)
    expect(retryAfterDeadline('bad', 1000)).toBe(1000)
  })
})
