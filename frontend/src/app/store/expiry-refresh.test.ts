import { describe, expect, it } from 'vitest'
import { expiryBoundaryCrossed } from './expiry-refresh'

describe('clock notifications without recurring REST refresh', () => {
  const expires = Date.parse('2026-10-14T12:00:00Z')
  const data = { items: [{ expires_at: '2026-10-14T12:00:00Z', remaining_seconds: 3600 }] }
  it('ignores ticks which change neither availability nor expiry warning', () => {
    expect(expiryBoundaryCrossed(data, expires - 30_000, 0, 24)).toBe(false)
  })
  it('refreshes at expiry once, even if the old read has not completed yet', () => {
    expect(expiryBoundaryCrossed(data, expires, 0, 24)).toBe(true)
    expect(expiryBoundaryCrossed(data, expires + 30_000, expires, 24)).toBe(false)
  })
  it('refreshes on crossing the restaurant warning threshold', () => {
    expect(expiryBoundaryCrossed({ items: [{ ...data.items[0], remaining_seconds: 7200 }] }, expires - 3600_000, 0, 1)).toBe(true)
  })
})
