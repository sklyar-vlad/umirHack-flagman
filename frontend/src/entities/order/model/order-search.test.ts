import { describe, expect, it } from 'vitest'
import { defaultOrderSearch, readOrderSearch, orderSearchParams, changeOrderSearch, orderSearchError, safeOrderListUrl } from './order-search'
import { safeReturnUrl } from '@/features/auth/model/return-url'

describe('independent order URL state', () => {
  it('round-trips both tabs, exact period, literal search and independent pages', () => {
    const values = { tab: 'attempts' as const, orders: { ...defaultOrderSearch.orders, search: '%_\\ суп', page: 3, from: '2026-10-10T00:00:00.123456789Z' }, attempts: { ...defaultOrderSearch.attempts, reason_code: 'RECIPE_CHANGED' as const, page: 2, page_size: 20 } }
    expect(readOrderSearch(orderSearchParams(values))).toEqual(values)
    expect(safeReturnUrl(`/orders?${orderSearchParams(values)}&password=secret#token`)).toBe(`/orders?${orderSearchParams(values)}`)
  })
  it('resets only active page on filter changes and excludes reason from successful orders', () => {
    const values = { ...defaultOrderSearch, orders: { ...defaultOrderSearch.orders, page: 4 }, attempts: { ...defaultOrderSearch.attempts, page: 2 } }
    expect(changeOrderSearch(values, { search: 'суп', reason_code: 'RECIPE_CHANGED' })).toEqual({ ...values, orders: { ...values.orders, page: 1, search: 'суп' } })
    expect(changeOrderSearch({ ...values, tab: 'attempts' }, { page: 3 }).orders).toEqual(values.orders)
  })
  it('rejects unknown reasons and invalid bounds without losing inactive state', () => {
    const values = readOrderSearch(new URLSearchParams('tab=attempts&attempts_reason_code=NETWORK_ERROR&orders_page=3'))
    expect(orderSearchError(values.attempts)).toContain('причина')
    expect(safeOrderListUrl('/orders?tab=attempts&attempts_reason_code=NETWORK_ERROR&orders_page=3')).toBe('/orders?tab=attempts&orders_page=3')
    expect(orderSearchError({ ...values.orders, from: '2026-02-30T00:00:00Z' })).toBeTruthy()
  })
  it.each(['https://evil.test/orders', '//evil.test', '/orders/new', '/orders/../login'])('restricts return links: %s', (value) => expect(safeOrderListUrl(value)).toBe('/orders'))
  it('normalizes unsafe pages and preserves approved entry return URLs through login', () => {
    expect(readOrderSearch(new URLSearchParams('orders_page=NaN&attempts_page_size=101')).orders).toEqual(defaultOrderSearch.orders)
    expect(safeReturnUrl('/orders/new?returnTo=%2Forders%3Ftab%3Dattempts%26attempts_page%3D2&token=secret')).toBe('/orders/new?returnTo=%2Forders%3Ftab%3Dattempts%26attempts_page%3D2')
  })
})
