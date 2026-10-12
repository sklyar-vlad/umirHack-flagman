import { readForecastSearch, forecastSearchParams, forecastSearchError, defaultForecastSearch } from '@/entities/stop-list-forecast'
import { readShortageSearch, shortageSearchParams, shortageSearchError, defaultShortageSearch } from '@/entities/ingredient-shortage'
import { readLossSearch, lossSearchParams, lossSearchError, defaultLossSearch } from '@/entities/write-off-report'
import { readMovementSearch, movementSearchParams, movementSearchError, defaultMovementSearch } from '@/entities/stock-movement'
import { readAuditSearch, auditSearchParams, auditSearchError, defaultAuditSearch } from '@/entities/audit-log'
import { readUserSearch, userSearchParams, safeUserListUrl } from '@/entities/user'
import { readProductSearch, productSearchParams, safeProductListUrl } from '@/entities/product'
import { z } from 'zod'
import { procurementSearchParams, readProcurementSearch } from '@/entities/procurement'
import { inventorySearchParams, readInventorySearch, inventorySelectionParams } from '@/entities/inventory'
import { safeOrderListUrl } from '@/entities/order'
import { safeSupplyListUrl } from '@/entities/supply'
import { readDishSearch, dishSearchParams, safeDishListUrl, readStopListSearch, stopListSearchParams, readHotOfferSearch, hotOfferSearchParams } from '@/entities/dish'

/** Reject external URLs, ambiguous encodings, login loops and credential-bearing query strings. */
export function safeReturnUrl(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || /[\\\s%]/u.test(value.split(/[?#]/)[0])) return '/'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || /^\/login(?:\/|$)/i.test(url.pathname)) return '/'
  if (url.pathname === '/analytics/stop-list-forecast' || url.pathname === '/forecast') {
    const values = readForecastSearch(url.searchParams)
    const params = forecastSearchParams(forecastSearchError(values) ? defaultForecastSearch : values)
    return '/analytics/stop-list-forecast' + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/analytics/ingredient-shortages') {
    const values = readShortageSearch(url.searchParams)
    const params = shortageSearchParams(shortageSearchError(values) ? defaultShortageSearch : values)
    return url.pathname + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/analytics' || url.pathname === '/analytics/write-offs') {
    const values = readLossSearch(url.searchParams)
    const params = lossSearchParams(lossSearchError(values) ? defaultLossSearch : values)
    return '/analytics/write-offs' + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/stock-movements' || url.pathname === '/movements') {
    const values = readMovementSearch(url.searchParams)
    const params = movementSearchParams(movementSearchError(values) ? defaultMovementSearch : values)
    return '/stock-movements' + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/admin/audit' || url.pathname === '/admin/audit-log') {
    const values = readAuditSearch(url.searchParams)
    const params = auditSearchParams(auditSearchError(values) ? defaultAuditSearch : values)
    return '/admin/audit-log' + (params.size ? `?${params}` : '')
  }
  if (url.pathname === '/admin/users') {
    const params = userSearchParams(readUserSearch(url.searchParams))
    return url.pathname + (params.size ? `?${params}` : '')
  }
  if (/^\/admin\/users\/(?:new|[0-9a-f-]{36}(?:\/(?:edit|password))?)$/i.test(url.pathname) && url.searchParams.has('returnTo')) {
    return `${url.pathname}?${new URLSearchParams({ returnTo: safeUserListUrl(url.searchParams.get('returnTo')) })}`
  }
  if (url.pathname === '/admin/dishes' || url.pathname === '/admin/recipes') {
    const params = dishSearchParams(readDishSearch(url.searchParams))
    return '/admin/dishes' + (params.size ? `?${params}` : '')
  }
  if (/^\/admin\/dishes\/(?:new|[0-9a-f-]{36}(?:\/edit)?)$/i.test(url.pathname) && url.searchParams.has('returnTo')) {
    return `${url.pathname}?${new URLSearchParams({ returnTo: safeDishListUrl(url.searchParams.get('returnTo')) })}`
  }
  if (url.pathname === '/admin/products') {
    const params = productSearchParams(readProductSearch(url.searchParams))
    return url.pathname + (params.size ? `?${params}` : '')
  }
  if (/^\/admin\/products\/(?:new|[0-9a-f-]{36}(?:\/edit)?)$/i.test(url.pathname) && url.searchParams.has('returnTo')) {
    return `${url.pathname}?${new URLSearchParams({ returnTo: safeProductListUrl(url.searchParams.get('returnTo')) })}`
  }
  // Published warehouse filters and F07 UI-only product/supply selection.
  if (url.pathname === '/inventory') {
    const allowed = inventorySearchParams(readInventorySearch(url.searchParams))
    inventorySelectionParams(url.searchParams).forEach((value, key) => allowed.set(key, value))
    const params = allowed.toString()
    return url.pathname + (params ? `?${params}` : '')
  }
  if (url.pathname === '/recommendations/offers' || url.pathname === '/recommendations' && url.searchParams.get('tab') === 'hot-offers') {
    const params = hotOfferSearchParams(readHotOfferSearch(url.searchParams))
    if (url.pathname === '/recommendations') params.set('tab', 'hot-offers')
    return url.pathname + (params.size ? `?${params}` : '')
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
