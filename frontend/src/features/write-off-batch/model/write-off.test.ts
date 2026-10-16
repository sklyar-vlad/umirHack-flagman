import { describe, expect, it } from 'vitest'
import { inventoryBatches, inventoryItems } from '@/app/mocks/inventory-fixtures'
import { writeOffRequestSchema, type WriteOffDraft } from '@/shared/api/write-off-work'
import { canRetryWriteOff, uncertainWriteOff, writeOffField, writeOffFormSchema, writeOffPayload } from './write-off'

const snapshot: WriteOffDraft = { batch: inventoryBatches[0], product: inventoryItems[0].product, values: { quantity: '', reason: '' } }
describe('exact published batch write-off validation', () => {
  it('converts six display decimals to exact base thousandths and trims Unicode reason', () => {
    expect(writeOffPayload({ quantity: '0,001125', reason: '  Повреждено 🥛  ' }, snapshot)).toEqual({ batch_id: snapshot.batch.id, quantity: '1.125', reason: 'Повреждено 🥛' })
  })
  it.each(['0', '-1', '1e-3', '0.0000001', '0.200001', 'Infinity', 'abc'])('refuses invalid or over-stock quantity %s', (quantity) => {
    expect(writeOffFormSchema(snapshot).safeParse({ quantity, reason: 'Порча' }).success).toBe(false)
  })
  it('allows the complete expired stock and inactive product without requiring availability', () => {
    const expired = { ...snapshot, batch: inventoryBatches[23], product: inventoryItems[1].product }
    expect(writeOffPayload({ quantity: '0.3', reason: 'Просрочка' }, expired).quantity).toBe('300.000')
  })
  it('allows only integral pcs', () => {
    const pcs = { ...snapshot, batch: inventoryBatches.at(-1)!, product: inventoryItems[4].product }
    expect(writeOffFormSchema(pcs).safeParse({ quantity: '1,5', reason: 'Порча' }).success).toBe(false)
    expect(writeOffPayload({ quantity: '10', reason: 'Порча' }, pcs).quantity).toBe('10.000')
  })
  it.each(['', '   ', 'x'.repeat(501), 'a\nb', 'a\u0000b'])('rejects invalid reason %j', (reason) => {
    expect(writeOffFormSchema(snapshot).safeParse({ quantity: '0.001', reason }).success).toBe(false)
  })
  it('counts Unicode code points at the published boundary', () => {
    expect(writeOffPayload({ quantity: '0.001', reason: '🥛'.repeat(500) }, snapshot).reason).toHaveLength(1000)
    expect(writeOffFormSchema(snapshot).safeParse({ quantity: '0.001', reason: '🥛'.repeat(501) }).success).toBe(false)
  })
  it('keeps strict DTO fields and never maps unknown server paths into form errors', () => {
    expect(writeOffRequestSchema.safeParse({ batch_id: snapshot.batch.id, quantity: '1.125', reason: 'Порча', remaining_quantity: '100' }).success).toBe(false)
    expect(writeOffField('quantity')).toBe('quantity'); expect(writeOffField('reason')).toBe('reason'); expect(writeOffField('batch_id')).toBeNull()
  })
  it.each([401, 403, 408, 429, 500, 'NETWORK_ERROR', 'INVALID_RESPONSE', 'TIMEOUT_ERROR'] as const)('locks immutable retry for %s', (status) => {
    expect(uncertainWriteOff({ status, code: 'ERROR', message: '', details: {} })).toBe(true)
  })
  it('distinguishes definitive stock/field refusals and concurrent or conflicting requests', () => {
    const error = { status: 409, code: 'INSUFFICIENT_BATCH_STOCK', message: '', details: {} }
    expect(uncertainWriteOff(error)).toBe(false)
    expect(uncertainWriteOff({ ...error, status: 422 })).toBe(false)
    expect(uncertainWriteOff({ ...error, code: 'REQUEST_IN_PROGRESS' })).toBe(true)
    expect(uncertainWriteOff({ ...error, code: 'IDEMPOTENCY_CONFLICT' })).toBe(true)
  })
  it('refuses retry after the safety window or clock reversal', () => {
    expect(canRetryWriteOff(1000, 1001)).toBe(true)
    expect(canRetryWriteOff(1000, 999)).toBe(false)
    expect(canRetryWriteOff(1000, 1000 + 23 * 3600000)).toBe(false)
  })
})
