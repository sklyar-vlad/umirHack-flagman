import { z } from 'zod'
import { procurementSearchParams, readProcurementSearch } from '@/entities/procurement'
import { inventorySearchParams, readInventorySearch, inventorySelectionParams } from '@/entities/inventory'
import { safeOrderListUrl } from '@/entities/order'
import { safeSupplyListUrl } from '@/entities/supply'
import { readStopListSearch, stopListSearchParams } from '@/entities/dish'

/** Reject external URLs, ambiguous encodings, login loops and credential-bearing query strings. */
export function safeReturnUrl(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || /[\\\s%]/u.test(value.split(/[?#]/)[0])) return '/'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || /^\/login(?:\/|$)/i.test(url.pathname)) return '/'
  // Published warehouse filters and F07 UI-only product/supply selection.
  if (url.pathname === '/inventory') {
    const allowed = inventorySearchParams(readInventorySearch(url.searchParams))
    inventorySelectionParams(url.searchParams).forEach((value, key) => allowed.set(key, value))
    const params = allowed.toString()
    return url.pathname + (params ? `?${params}` : '')
  }
  if (url.pathname === '/recommendations/procurement' || url.pathname === '/recommendations' && url.searchParams.get('tab') === 'procurement') {
    const params = procurementSearchParams(readProcurementSearch(url.searchParams))
    if (url.pathname === '/recommendations') params.set('tab', 'procurement')
    return url.pathname + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/supplies/new' && z.uuid().safeParse(url.searchParams.get('product_id')).success) {
    const params = new URLSearchParams({ product_id: url.searchParams.get('product_id')! })
    if (url.searchParams.has('returnTo')) params.set('returnTo', safeSupplyListUrl(url.searchParams.get('returnTo')))
    return `${url.pathname}?${params}`
  }
  if (url.pathname === '/orders') return safeOrderListUrl(url.pathname + url.search)
  if (url.pathname === '/stop-list') {
    const params = stopListSearchParams(readStopListSearch(url.searchParams)).toString()
    return url.pathname + (params ? `?${params}` : '')
  }
  if (/^\/orders\/(?:new|[0-9a-f-]{36})$/i.test(url.pathname) && url.searchParams.has('returnTo')) {
    return `${url.pathname}?returnTo=${encodeURIComponent(safeOrderListUrl(url.searchParams.get('returnTo')))}`
  }
  if (url.pathname === '/supplies') return safeSupplyListUrl(url.pathname + url.search)
  if (/^\/supplies\/(?:new|[0-9a-f-]{36})$/i.test(url.pathname) && url.searchParams.has('returnTo')) {
    return `${url.pathname}?returnTo=${encodeURIComponent(safeSupplyListUrl(url.searchParams.get('returnTo')))}`
  }
  return url.pathname
}
export function loginUrl(value: string): string {
  const target = safeReturnUrl(value)
  return target === '/' ? '/login' : `/login?returnTo=${encodeURIComponent(target)}`
}
