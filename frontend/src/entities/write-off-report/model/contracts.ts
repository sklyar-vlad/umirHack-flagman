import { z } from 'zod'
import { quantityToInput } from '@/shared/lib/quantity'

export const aggregateQuantitySchema = z.string().regex(/^\d+(?:\.\d{1,3})?$/)
export function lossQuantity(raw: string): bigint {
  const [whole, fraction = ''] = raw.split('.')
  return BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, '0'))
}
export const lossSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  expired_written_off_quantity: aggregateQuantitySchema, manual_written_off_quantity: aggregateQuantitySchema,
  total_written_off_quantity: aggregateQuantitySchema, movements_count: z.number().int().positive().safe(),
}).strict().refine((row) => {
  try {
    const quantities = [row.expired_written_off_quantity, row.manual_written_off_quantity, row.total_written_off_quantity]
    quantities.forEach((raw) => quantityToInput(raw, row.unit))
    return lossQuantity(quantities[0]) + lossQuantity(quantities[1]) === lossQuantity(quantities[2]) && lossQuantity(quantities[2]) > 0n
  } catch { return false }
}, 'Некорректный итог или единица потерь')
const integer = z.number().int().safe()
export const lossReportSchema = z.object({ items: z.array(lossSchema), total: integer.nonnegative(), page: integer.positive(), page_size: integer.min(1).max(100), meta: z.object({}) }).strict()
export type LossRow = z.infer<typeof lossSchema>
