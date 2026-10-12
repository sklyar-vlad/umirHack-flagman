import { z } from 'zod'
import type { ApiError } from '@/shared/api'
import { writeOffValuesSchema, type WriteOffDraft, type WriteOffValues, type WriteOffRequest } from '@/shared/api/write-off-work'
import { parseQuantity, quantityFromInput } from '@/shared/lib/quantity'

export function writeOffFormSchema(snapshot: Pick<WriteOffDraft, 'batch' | 'product'>) {
  return writeOffValuesSchema.superRefine((values, ctx) => {
    try {
      const quantity = quantityFromInput(values.quantity, snapshot.batch.unit, snapshot.product.display_unit)
      if (parseQuantity(quantity)! > parseQuantity(snapshot.batch.remaining_quantity)!) throw new Error('Количество превышает показанный физический остаток партии.')
    } catch (reason) { ctx.addIssue({ code: 'custom', path: ['quantity'], message: reason instanceof Error ? reason.message : 'Проверьте количество.' }) }
    const reason = values.reason.trim()
    if ([...reason].length < 1 || [...reason].length > 500 || /[\p{Cc}]/u.test(reason)) ctx.addIssue({ code: 'custom', path: ['reason'], message: 'Укажите причину: 1–500 символов без управляющих символов.' })
  })
}
export function writeOffPayload(values: WriteOffValues, snapshot: Pick<WriteOffDraft, 'batch' | 'product'>): WriteOffRequest {
  writeOffFormSchema(snapshot).parse(values)
  return { batch_id: snapshot.batch.id, quantity: quantityFromInput(values.quantity, snapshot.batch.unit, snapshot.product.display_unit), reason: values.reason.trim() }
}
export const writeOffField = (path: string): keyof WriteOffValues | null => path === 'quantity' || path === 'reason' ? path : null
export const writeOffViolations = z.array(z.object({ path: z.string(), code: z.string(), message: z.string() }))
/** Access/rate errors can occur before a saved replay result is read. */
export function uncertainWriteOff(error: ApiError) {
  return typeof error.status !== 'number' || error.status >= 500 || [401, 403, 408, 429].includes(error.status) || ['IDEMPOTENCY_CONFLICT', 'REQUEST_IN_PROGRESS', 'SESSION_CHANGED'].includes(error.code)
}
/** A safety margin inside the published 24-hour idempotency retention. */
export function canRetryWriteOff(startedAt: number, now = Date.now()) { return now >= startedAt && now - startedAt < 23 * 3600000 }
