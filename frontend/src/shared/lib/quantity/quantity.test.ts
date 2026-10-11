import { describe, expect, it } from 'vitest'
import {
  MAX_QUANTITY, QuantityError, formatMovementQuantity, formatQuantity,
  parseMovementQuantity, parseQuantity, quantityFromInput, quantityToInput,
  serializeMovementQuantity, serializeQuantity,
} from './index'

describe('exact API quantities', () => {
  it.each([
    ['12', 12000n, '12.000'], ['12.5', 12500n, '12.500'],
    ['0.001', 1n, '0.001'], ['0', 0n, '0.000'],
    ['99999999999.999', MAX_QUANTITY, '99999999999.999'],
  ])('round trips %s through thousandths', (raw, value, canonical) => {
    expect(parseQuantity(raw)).toBe(value)
    expect(serializeQuantity(value)).toBe(canonical)
  })

  it('preserves null separately from a zero target stock', () => {
    expect(parseQuantity(null)).toBeNull()
    expect(serializeQuantity(null)).toBeNull()
    expect(quantityToInput(null, 'g', 'kg')).toBeNull()
    expect(formatQuantity(null, 'g', 'kg')).toBe('—')
    expect(formatQuantity('0.000', 'g', 'kg')).toBe('0 кг')
  })

  it.each(['', ' 1', '1 ', '1\n', '-1', '+1', '1,25', '1e3', '.1', '1.', '1.0001', '100000000000', '000000000001'])(
    'rejects invalid API quantity %j without rounding', (raw) => {
      expect(() => parseQuantity(raw)).toThrow(QuantityError)
    },
  )

  it('enforces serialization bounds and keeps decimal math exact', () => {
    expect(serializeQuantity(parseQuantity('0.100')! + parseQuantity('0.200')!)).toBe('0.300')
    expect(() => serializeQuantity(MAX_QUANTITY + 1n)).toThrow(expect.objectContaining({ code: 'out_of_range' }))
    expect(() => serializeQuantity(-1n)).toThrow(QuantityError)
  })
})

describe('compatible display units and positive form input', () => {
  it('converts kg to base grams and back without losing the smallest fraction', () => {
    expect(quantityFromInput('1,000001', 'g', 'kg')).toBe('1000.001')
    expect(quantityToInput('1000.001', 'g', 'kg')).toBe('1.000001')
    expect(formatQuantity('1000.001', 'g', 'kg')).toBe('1,000001 кг')
    expect(quantityFromInput('0.000001', 'g', 'kg')).toBe('0.001')
  })

  it('converts liters to milliliters and allows surrounding form whitespace', () => {
    expect(quantityFromInput(' 2,5 ', 'ml', 'l')).toBe('2500.000')
    expect(formatQuantity('2500.000', 'ml', 'l')).toBe('2,5 л')
    expect(formatQuantity('2.500', 'ml')).toBe('2,5 мл')
    expect(formatQuantity('120.000', 'g')).toBe('120 г')
  })

  it('checks numeric(14,3) after converting the display unit', () => {
    expect(quantityFromInput('99999999.999999', 'g', 'kg')).toBe('99999999999.999')
    expect(() => quantityFromInput('100000000', 'g', 'kg')).toThrow(expect.objectContaining({ code: 'out_of_range' }))
    expect(() => quantityFromInput('0.0000001', 'g', 'kg')).toThrow(QuantityError)
    expect(() => quantityFromInput('1.0001', 'g')).toThrow(QuantityError)
  })

  it('does not apply the single-quantity limit to an inventory aggregate', () => {
    expect(formatQuantity('199999999999.998', 'g', 'kg')).toBe('199999999,999998 кг')
    expect(quantityToInput('999999999999999999999.001', 'ml', 'l')).toBe('999999999999999999.999001')
    expect(() => quantityFromInput('199999999.999998', 'g', 'kg')).toThrow(QuantityError)
  })

  it('distinguishes positive operations from nonnegative target stock', () => {
    expect(() => quantityFromInput('0', 'g')).toThrow(expect.objectContaining({ code: 'not_positive' }))
    expect(quantityFromInput('0', 'g', 'kg', { allowZero: true })).toBe('0.000')
    for (const raw of ['-1', '+1', '1e3', '', '1,2.3']) {
      expect(() => quantityFromInput(raw, 'g')).toThrow(QuantityError)
    }
  })

  it('requires whole pieces and compatible unit pairs', () => {
    expect(quantityFromInput('12.000', 'pcs')).toBe('12.000')
    expect(formatQuantity('12.000', 'pcs')).toBe('12 шт.')
    expect(() => quantityFromInput('0.001', 'pcs')).toThrow(expect.objectContaining({ code: 'fractional_pieces' }))
    expect(() => formatQuantity('1.001', 'pcs')).toThrow(QuantityError)
    expect(() => quantityFromInput('1', 'g', 'l')).toThrow(expect.objectContaining({ code: 'incompatible_unit' }))
    expect(() => quantityToInput('1', 'ml', 'kg')).toThrow(QuantityError)
    expect(() => quantityFromInput('1', 'pcs', 'kg')).toThrow(QuantityError)
  })
})

describe('signed stock movements', () => {
  it('represents receipt and consumption separately from positive inputs', () => {
    expect(parseMovementQuantity('-0.001')).toBe(-1n)
    expect(serializeMovementQuantity(-MAX_QUANTITY)).toBe('-99999999999.999')
    expect(formatMovementQuantity('-1200.500', 'g', 'kg')).toBe('-1,2005 кг')
    expect(formatMovementQuantity('1000.000', 'ml', 'l')).toBe('+1 л')
    expect(formatMovementQuantity('-0.000', 'pcs')).toBe('0 шт.')
    expect(() => quantityFromInput('-1200.500', 'g')).toThrow(QuantityError)
  })

  it('preserves null and validates movement precision and bounds', () => {
    expect(parseMovementQuantity(null)).toBeNull()
    expect(serializeMovementQuantity(null)).toBeNull()
    expect(formatMovementQuantity(null, 'g')).toBe('—')
    for (const raw of ['+1', '-100000000000', '-0.0001', '-1\n']) {
      expect(() => parseMovementQuantity(raw)).toThrow(QuantityError)
    }
    expect(() => serializeMovementQuantity(-MAX_QUANTITY - 1n)).toThrow(QuantityError)
    expect(() => formatMovementQuantity('-1.001', 'pcs')).toThrow(QuantityError)
  })
})
