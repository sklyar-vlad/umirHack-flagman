import { inventorySearchParams, readInventorySearch } from '@/entities/inventory'

/** Reject external URLs, ambiguous encodings, login loops and credential-bearing query strings. */
export function safeReturnUrl(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || /[\\\s%]/u.test(value.split(/[?#]/)[0])) return '/'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || /^\/login(?:\/|$)/i.test(url.pathname)) return '/'
  // F05 opts in only the published, non-secret warehouse filters.
  if (url.pathname === '/inventory') {
    const params = inventorySearchParams(readInventorySearch(url.searchParams)).toString()
    return url.pathname + (params ? `?${params}` : '')
  }
  return url.pathname
}
export function loginUrl(value: string): string {
  const target = safeReturnUrl(value)
  return target === '/' ? '/login' : `/login?returnTo=${encodeURIComponent(target)}`
}
