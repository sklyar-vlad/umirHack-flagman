import { z } from 'zod'
import { compareSupplyInstants } from '@/entities/supply'
import type { Product } from '@/entities/product'
import { receptionValuesSchema, type CreateSupplyRequest, type ReceptionValues, type ApiError } from '@/shared/api'
import { quantityFromInput } from '@/shared/lib/quantity'
import { restaurantDateTimeFromInput, restaurantDateTimeToInput } from '@/shared/lib/restaurant-time'

export const emptyReceptionLine = () => ({ productId: '', quantity: '', displayUnit: 'kg' as const, expiresAt: '' })
export const emptyReception = (): ReceptionValues => ({ number: '', receivedAt: '', items: [emptyReceptionLine()] })
/** Explicit review preserves instants; immutable pending request bodies are separate. */
export function convertReceptionTimezone(values: ReceptionValues, from: string, to: string): ReceptionValues {
  const convert = (value: string) => {
    if (!value) return ''
    const instant = restaurantDateTimeFromInput(value, from)!
    const projected = restaurantDateTimeToInput(instant, to)!
    // An overlap in the destination zone cannot be represented unambiguously by datetime-local.
    // Reject the entire conversion before resetting/persisting any original date.
    if (compareSupplyInstants(restaurantDateTimeFromInput(projected, to)!, instant) !== 0) throw new Error('Не удалось сохранить исходный момент.')
    return projected
  }
  return { ...values, receivedAt: convert(values.receivedAt), items: values.items.map((line) => ({ ...line, expiresAt: convert(line.expiresAt) })) }
}
export function receptionSchema(products: Product[], timezone: string) {
  return receptionValuesSchema.superRefine((values, ctx) => {
    const issue = (path: (string | number)[], message: string) => ctx.addIssue({ code: 'custom', path, message })
    if (!values.number.trim()) issue(['number'], 'Укажите номер поставки.')
    let received: string | null = null
    try { received = restaurantDateTimeFromInput(values.receivedAt, timezone); if (!received) throw new Error('Укажите дату приёмки.') } catch (error) { issue(['receivedAt'], error instanceof Error ? error.message : 'Некорректная дата.') }
    values.items.forEach((line, index) => {
      const product = products.find((item) => item.id === line.productId && item.is_active)
      if (!product) issue(['items', index, 'productId'], 'Выберите активный продукт.')
      if (product) try { quantityFromInput(line.quantity, product.unit, line.displayUnit) } catch (error) { issue(['items', index, 'quantity'], error instanceof Error ? error.message : 'Некорректное количество.') }
      try {
        const expires = restaurantDateTimeFromInput(line.expiresAt, timezone)
        if (!expires) throw new Error('Укажите срок годности.')
        if (received && compareSupplyInstants(expires, received) <= 0) throw new Error('Срок годности должен быть позже приёмки.')
      } catch (error) { issue(['items', index, 'expiresAt'], error instanceof Error ? error.message : 'Некорректная дата.') }
    })
  })
}
export function receptionPayload(values: ReceptionValues, products: Product[], timezone: string): CreateSupplyRequest {
  receptionSchema(products, timezone).parse(values)
  return { number: values.number.trim(), received_at: restaurantDateTimeFromInput(values.receivedAt, timezone)!, items: values.items.map((line) => {
    const product = products.find((item) => item.id === line.productId)!
    return { product_id: product.id, quantity: quantityFromInput(line.quantity, product.unit, line.displayUnit), expires_at: restaurantDateTimeFromInput(line.expiresAt, timezone)! }
  }) }
}
export type ReceptionField = 'number' | 'receivedAt' | `items.${number}.productId` | `items.${number}.quantity` | `items.${number}.expiresAt`
export function receptionField(path: string, count: number): ReceptionField | null {
  if (path === 'number') return 'number'
  if (path === 'received_at') return 'receivedAt'
  const match = /^items\[(\d+)\]\.(product_id|quantity|expires_at)$/.exec(path)
  if (!match || Number(match[1]) >= count) return null
  const fields = { product_id: 'productId', quantity: 'quantity', expires_at: 'expiresAt' } as const
  return `items.${Number(match[1])}.${fields[match[2] as keyof typeof fields]}`
}
export function uncertainReception(error: ApiError) {
  // Access/rate checks can reject a replay before reading its saved result.
  return typeof error.status !== 'number' || error.status >= 500 || [401, 403, 408, 429].includes(error.status) || error.code === 'IDEMPOTENCY_CONFLICT' || error.code === 'REQUEST_IN_PROGRESS' || error.code === 'SESSION_CHANGED'
}
export const fieldViolationsSchema = z.array(z.object({ path: z.string(), code: z.string(), message: z.string() }))
/** Server promises at least 24h retention. Never replay automatically beyond that safety window. */
export function canRetryReception(startedAt: number, now = Date.now()) { return now >= startedAt && now - startedAt < 23 * 60 * 60 * 1000 }
