export type Unit = 'g' | 'ml' | 'pcs'
export type DisplayUnit = Unit | 'kg' | 'l'
export type QuantityErrorCode = 'invalid_format' | 'out_of_range' | 'not_positive' | 'fractional_pieces' | 'incompatible_unit'

export class QuantityError extends Error {
  constructor(public readonly code: QuantityErrorCode, message: string) {
    super(message)
    this.name = 'QuantityError'
  }
}

export const MAX_QUANTITY = 99_999_999_999_999n
const SCALE = 1000n
const labels: Record<DisplayUnit, string> = { g: 'г', kg: 'кг', ml: 'мл', l: 'л', pcs: 'шт.' }

function parseDecimal(raw: string, signed = false, bounded = true): bigint {
  const pattern = signed ? /^-?\d{1,11}(?:\.\d{1,3})?$/ : bounded
    ? /^\d{1,11}(?:\.\d{1,3})?$/ : /^\d+(?:\.\d{1,3})?$/
  if (raw !== raw.trim() || !pattern.test(raw)) {
    throw new QuantityError('invalid_format', 'Ожидается точное количество с максимум тремя знаками после точки.')
  }
  const negative = raw.startsWith('-')
  const [whole, fraction = ''] = (negative ? raw.slice(1) : raw).split('.')
  const value = BigInt(whole) * SCALE + BigInt(fraction.padEnd(3, '0'))
  return negative ? -value : value
}

export function parseQuantity(raw: string | null): bigint | null {
  return raw === null ? null : parseDecimal(raw)
}

export function parseMovementQuantity(raw: string | null): bigint | null {
  return raw === null ? null : parseDecimal(raw, true)
}

function decimalString(value: bigint, precision: number): string {
  const magnitude = value < 0n ? -value : value
  const digits = magnitude.toString().padStart(precision + 1, '0')
  return `${value < 0n ? '-' : ''}${digits.slice(0, -precision)}.${digits.slice(-precision)}`
}

function serialize(value: bigint | null, signed: boolean): string | null {
  if (value === null) return null
  if (value > MAX_QUANTITY || value < (signed ? -MAX_QUANTITY : 0n)) {
    throw new QuantityError('out_of_range', 'Количество выходит за границы numeric(14,3).')
  }
  return decimalString(value, 3)
}

export function serializeQuantity(value: bigint | null): string | null {
  return serialize(value, false)
}

export function serializeMovementQuantity(value: bigint | null): string | null {
  return serialize(value, true)
}

function displayPrecision(unit: Unit, displayUnit: DisplayUnit): number {
  if (unit === displayUnit) return 3
  if ((unit === 'g' && displayUnit === 'kg') || (unit === 'ml' && displayUnit === 'l')) return 6
  throw new QuantityError('incompatible_unit', 'Единица отображения несовместима с базовой единицей продукта.')
}

function checkPieces(value: bigint, unit: Unit) {
  if (unit === 'pcs' && value % SCALE !== 0n) {
    throw new QuantityError('fractional_pieces', 'Количество штук должно быть целым.')
  }
}

function displayValue(value: bigint, unit: Unit, displayUnit: DisplayUnit): string {
  const precision = displayPrecision(unit, displayUnit)
  checkPieces(value, unit)
  return decimalString(value, precision).replace(/\.?0+$/, '')
}

export function quantityToInput(raw: string | null, unit: Unit, displayUnit: DisplayUnit = unit): string | null {
  return raw === null ? null : displayValue(parseDecimal(raw, false, false), unit, displayUnit)
}

export function quantityFromInput(
  raw: string,
  unit: Unit,
  displayUnit: DisplayUnit = unit,
  { allowZero = false }: { allowZero?: boolean } = {},
): string {
  const precision = displayPrecision(unit, displayUnit)
  const pattern = new RegExp(`^\\d{1,11}(?:[.,]\\d{1,${precision}})?$`)
  if (!pattern.test(raw.trim())) {
    throw new QuantityError('invalid_format', `Ожидается количество с максимум ${precision} знаками после разделителя.`)
  }
  const [whole, fraction = ''] = raw.trim().replace(',', '.').split('.')
  const value = BigInt(whole) * 10n ** BigInt(precision) + BigInt(fraction.padEnd(precision, '0'))
  checkPieces(value, unit)
  if (!allowZero && value === 0n) {
    throw new QuantityError('not_positive', 'Количество должно быть больше нуля.')
  }
  return serializeQuantity(value)!
}

export function formatQuantity(raw: string | null, unit: Unit, displayUnit: DisplayUnit = unit): string {
  const value = quantityToInput(raw, unit, displayUnit)
  return value === null ? '—' : `${value.replace('.', ',')} ${labels[displayUnit]}`
}

export function formatMovementQuantity(raw: string | null, unit: Unit, displayUnit: DisplayUnit = unit): string {
  const value = parseMovementQuantity(raw)
  if (value === null) return '—'
  return `${value > 0n ? '+' : ''}${displayValue(value, unit, displayUnit).replace('.', ',')} ${labels[displayUnit]}`
}
