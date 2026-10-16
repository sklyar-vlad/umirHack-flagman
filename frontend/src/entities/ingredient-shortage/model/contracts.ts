import { z } from 'zod'
import { quantityToInput } from '@/shared/lib/quantity'

export const shortageRowSchema = z.object({
  product_id: z.uuid(), product_name: z.string(), unit: z.enum(['g', 'ml', 'pcs']),
  rejected_attempts_count: z.number().int().positive().safe(),
  total_missing_quantity: z.string().regex(/^\d+(?:\.\d{1,3})?$/),
}).strict().refine((row) => {
  try { return quantityToInput(row.total_missing_quantity, row.unit) !== '0' } catch { return false }
}, 'Некорректное количество недостачи')
const integer = z.number().int().safe()
export const shortageReportSchema = z.object({
  items: z.array(shortageRowSchema), total: integer.nonnegative(), page: integer.positive(),
  page_size: integer.min(1).max(100), meta: z.object({}),
}).strict()
export type ShortageRow = z.infer<typeof shortageRowSchema>
