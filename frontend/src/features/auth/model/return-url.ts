/** Reject external URLs, ambiguous encodings, login loops and credential-bearing query strings. */
export function safeReturnUrl(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || /[\\\s%]/u.test(value)) return '/'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || /^\/login(?:\/|$)/i.test(url.pathname)) return '/'
  // Retain a path only: future business filters must opt in to non-secret query fields.
  return url.pathname
}
export function loginUrl(value: string): string {
  const target = safeReturnUrl(value)
  return target === '/' ? '/login' : `/login?returnTo=${encodeURIComponent(target)}`
}
