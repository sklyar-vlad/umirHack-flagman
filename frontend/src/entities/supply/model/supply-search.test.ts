import { describe, expect, it } from 'vitest'
import { changeSupplySearch, compareSupplyInstants, defaultSupplySearch, readSupplySearch, safeSupplyListUrl, supplyFilterSchema, supplyInstantToInput, supplyPeriodError, supplySearchParams } from './supply-search'

describe('supply URL and restaurant period', () => {
  it('round-trips literal numbers and RFC3339 bounds without forwarding unknown fields', () => {
    const values = { ...defaultSupplySearch, search: '%_\\ №', from: '2026-10-01T00:00:00+03:00', to: '2026-11-01T00:00:00.123456789Z', page: 2, page_size: 20 }
    expect(readSupplySearch(supplySearchParams(values))).toEqual(values)
    expect(safeSupplyListUrl('/supplies?search=104&page=2&password=secret#token')).toBe('/supplies?search=104&page=2')
  })
  it.each(['page=-1&page_size=101&sort=name', 'page=1.2&page_size=0', 'page=9007199254740992&page_size=NaN'])('normalizes unsafe pagination and fixed sort: %s', (raw) => {
    expect(readSupplySearch(new URLSearchParams(raw))).toEqual(defaultSupplySearch)
  })
  it.each([{ search: '104' }, { from: '2026-01-01T00:00:00Z' }, { to: '2026-02-01T00:00:00Z' }, { page_size: 20 }])('resets page when a filter changes: %j', (patch) => {
    expect(changeSupplySearch({ ...defaultSupplySearch, page: 4 }, patch)).toMatchObject({ ...patch, page: 1 })
  })
  it('preserves filters on page navigation', () => {
    expect(changeSupplySearch({ ...defaultSupplySearch, search: '104' }, { page: 2 })).toMatchObject({ search: '104', page: 2 })
  })
  it.each(['https://evil.test/supplies', '//evil.test', '/supplies/new', '/supplies/../login', '/supplies?from=invalid&token=secret'])('restricts list-return URLs: %s', (raw) => {
    expect(safeSupplyListUrl(raw)).toBe('/supplies')
  })
  it('converts Moscow boundaries regardless of browser timezone and preserves fractional seconds', () => {
    const parsed = supplyFilterSchema('Europe/Moscow').parse({ search: '104', from: '2026-10-10T00:00', to: '2026-10-11T00:00:00.123456789' })
    expect(parsed).toEqual({ search: '104', from: '2026-10-09T21:00:00Z', to: '2026-10-10T21:00:00.123456789Z' })
  })
  it('accepts equal/one-sided boundaries and rejects inverted, absent timezone and impossible dates', () => {
    expect(supplyPeriodError({ from: '', to: '2026-10-11T00:00:00Z' })).toBeNull()
    expect(supplyPeriodError({ from: '2026-10-11T03:00:00+03:00', to: '2026-10-11T00:00:00Z' })).toBeNull()
    for (const from of ['2026-02-30T00:00:00Z', '2026-10-11T00:00:00', '2026-10-12T00:00:00Z']) expect(supplyPeriodError({ from, to: '2026-10-11T00:00:00Z' })).toBeTruthy()
    expect(supplyPeriodError({ from: '2026-10-11T00:00:00.000000002Z', to: '2026-10-11T00:00:00.000000001Z' })).toBeTruthy()
    expect(compareSupplyInstants('2026-10-11T03:00:00.000000001+03:00', '2026-10-11T00:00:00.000000002Z')).toBe(-1)
  })
  it.each(['2026-03-08T02:30', '2026-11-01T01:30'])('rejects DST gaps/overlaps: %s', (from) => {
    expect(supplyFilterSchema('America/New_York').safeParse({ search: '', from, to: '' }).success).toBe(false)
  })
  it('validates inverted local period on the end control and accepts clearing the period', () => {
    const schema = supplyFilterSchema('Europe/Moscow')
    const result = schema.safeParse({ search: '', from: '2026-10-11T00:00', to: '2026-10-10T00:00' })
    expect(result.success).toBe(false)
    if (!result.success) expect(result.error.issues[0].path).toEqual(['to'])
    expect(schema.parse({ search: '', from: '', to: '' })).toEqual({ search: '', from: '', to: '' })
  })
  it('preserves nanosecond bounds and unambiguous API instants during an unchanged native form submission', () => {
    const original = { from: '2026-10-10T00:00:00.123456789Z', to: '2026-10-11T00:00:00.120000001Z' }
    const timezone = 'Europe/Moscow'
    expect(supplyFilterSchema(timezone, original).parse({ search: 'new number', from: supplyInstantToInput(original.from, timezone), to: supplyInstantToInput(original.to, timezone) })).toEqual({ ...original, search: 'new number' })
    const ambiguous = { from: '2026-11-01T05:30:00Z', to: '' }
    expect(supplyFilterSchema('America/New_York', ambiguous).parse({ search: '104', from: '2026-11-01T01:30', to: '' })).toEqual({ ...ambiguous, search: '104' })
  })
})
