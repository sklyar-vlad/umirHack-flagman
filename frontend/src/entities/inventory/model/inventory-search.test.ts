import { describe, expect, it } from 'vitest'
import { changeInventorySearch, defaultInventorySearch, inventorySearchParams, readInventorySearch } from './inventory-search'

describe('inventory URL parameters', () => {
  it('round-trips every published filter, sort, page and literal search', () => {
    const values = { ...defaultInventorySearch, search: 'Молоко %_\\', category_id: '00000000-0000-4000-8000-000000000001', filter: 'expired' as const, sort: 'nearest_expires_at' as const, page: 3, page_size: 20 }
    expect(readInventorySearch(inventorySearchParams(values))).toEqual(values)
  })
  it.each(['filter=unknown&sort=-name&category_id=invalid&page=-1&page_size=101', 'page=1.2&page_size=0', 'page=9007199254740992&page_size=NaN'])('normalizes unsafe parameters without sending them to API: %s', (raw) => {
    expect(readInventorySearch(new URLSearchParams(raw))).toEqual(defaultInventorySearch)
  })
  it('does not forward unrelated or repeated URL values', () => {
    const params = inventorySearchParams(readInventorySearch(new URLSearchParams('filter=expired&filter=all&password=secret')))
    expect(params.toString()).toBe('filter=expired')
  })
  it.each([{ search: 'Яйцо' }, { category_id: '00000000-0000-4000-8000-000000000002' }, { filter: 'deficit' as const }, { sort: 'nearest_expires_at' as const }, { page_size: 20 }])('resets the page for a changed control: %j', (patch) => {
    expect(changeInventorySearch({ ...defaultInventorySearch, page: 4 }, patch)).toMatchObject({ ...patch, page: 1 })
  })
  it('preserves the search when navigating to a page', () => {
    expect(changeInventorySearch({ ...defaultInventorySearch, search: 'Молоко' }, { page: 2 })).toMatchObject({ search: 'Молоко', page: 2 })
  })
})
