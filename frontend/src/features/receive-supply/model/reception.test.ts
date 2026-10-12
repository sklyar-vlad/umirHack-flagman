import { describe, expect, it } from 'vitest'
import { receptionProducts } from '@/app/mocks/reception-fixtures'
import { createSupplyRequestSchema } from '@/shared/api'
import { convertReceptionTimezone, canRetryReception, receptionField, receptionPayload, receptionSchema, uncertainReception } from './reception'

const milk = receptionProducts.find((item) => item.name === 'Молоко')!, eggs = receptionProducts.find((item) => item.name === 'Яйцо')!
const values = { number: '  Н-101  ', receivedAt: '2026-10-11T12:00', items: [{ productId: milk.id, quantity: '1,000001', displayUnit: 'l' as const, expiresAt: '2026-10-12T12:00' }] }
describe('reception validation and exact wire body', () => {
  it('preserves all instants, including fractional seconds, on explicit zone review', () => {
    const source = { ...values, receivedAt: '2026-10-11T12:00:00.123456789' }
    const converted = convertReceptionTimezone(source, 'Europe/Moscow', 'Asia/Yekaterinburg')
    expect(converted.receivedAt).toBe('2026-10-11T14:00:00.123456789')
    expect(receptionPayload(converted, receptionProducts, 'Asia/Yekaterinburg')).toEqual(receptionPayload(source, receptionProducts, 'Europe/Moscow'))
    expect(() => convertReceptionTimezone({ ...values, receivedAt: '2026-03-08T02:30' }, 'America/New_York', 'UTC')).toThrow()
    expect(() => convertReceptionTimezone({ ...values, receivedAt: '2026-11-01T05:30' }, 'UTC', 'America/New_York')).toThrow()
  })
  it('converts restaurant wall time and six decimal display units exactly, preserving duplicate lines', () => {
    const body = receptionPayload({ ...values, items: [values.items[0], values.items[0]] }, receptionProducts, 'Europe/Moscow')
    expect(body).toEqual({ number: 'Н-101', received_at: '2026-10-11T09:00:00Z', items: Array(2).fill({ product_id: milk.id, quantity: '1000.001', expires_at: '2026-10-12T09:00:00Z' }) })
    expect(createSupplyRequestSchema.safeParse(body).success).toBe(true)
  })
  it.each(['0', '-1', '1.0000001', '100000000', 'NaN'])('rejects invalid display quantity %s', (quantity) => {
    expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, items: [{ ...values.items[0], quantity }] }).success).toBe(false)
  })
  it('permits max numeric quantity and whole pieces, rejects fractions and incompatible units', () => {
    expect(receptionPayload({ ...values, items: [{ ...values.items[0], quantity: '99999999.999999' }] }, receptionProducts, 'Europe/Moscow').items[0].quantity).toBe('99999999999.999')
    for (const [quantity, valid] of [['1.5', false], ['2', true]] as const) expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, items: [{ ...values.items[0], productId: eggs.id, displayUnit: 'pcs', quantity }] }).success).toBe(valid)
    expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, items: [{ ...values.items[0], displayUnit: 'kg' }] }).success).toBe(false)
  })
  it('enforces number, date, active product, strictly later expiry and 1–100 lines', () => {
    for (const patch of [{ number: ' ' }, { receivedAt: '' }, { items: [] }, { items: Array(101).fill(values.items[0]) }, { items: [{ ...values.items[0], productId: crypto.randomUUID() }] }, { items: [{ ...values.items[0], expiresAt: values.receivedAt }] }]) expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, ...patch }).success).toBe(false)
    expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, items: Array(100).fill(values.items[0]) }).success).toBe(true)
  })
  it('compares expiry at exact nanosecond precision', () => {
    expect(receptionSchema(receptionProducts, 'Europe/Moscow').safeParse({ ...values, receivedAt: '2026-10-11T12:00:00.000000001', items: [{ ...values.items[0], expiresAt: '2026-10-11T12:00:00.000000002' }] }).success).toBe(true)
  })
  it('rejects DST gaps/overlaps, uses the configured timezone', () => {
    expect(receptionSchema(receptionProducts, 'America/New_York').safeParse({ ...values, receivedAt: '2026-03-08T02:30' }).success).toBe(false)
    expect(receptionPayload(values, receptionProducts, 'Asia/Yekaterinburg').received_at).toBe('2026-10-11T07:00:00Z')
  })
  it('maps only original payload index fields and preserves the repeat safety window', () => {
    expect(receptionField('items[1].expires_at', 2)).toBe('items.1.expiresAt')
    expect(receptionField('items[2].quantity', 2)).toBeNull()
    expect(receptionField('__proto__.password', 2)).toBeNull()
    expect(receptionField('number', 2)).toBe('number')
    expect(canRetryReception(1000, 1001)).toBe(true)
    expect(canRetryReception(1000, 1000 + 24 * 3600000)).toBe(false)
    expect(canRetryReception(1001, 1000)).toBe(false)
  })
  it.each([['NETWORK_ERROR', '', true], [500, '', true], [401, '', true], [403, 'FORBIDDEN', true], [429, 'RATE_LIMITED', true], [409, 'REQUEST_IN_PROGRESS', true], [409, 'IDEMPOTENCY_CONFLICT', true], [409, 'CONFLICT', false], [422, 'VALIDATION_ERROR', false]] as const)('classifies %s/%s', (status, code, unresolved) => {
    expect(uncertainReception({ status, code, message: '', details: {} })).toBe(unresolved)
  })
})
