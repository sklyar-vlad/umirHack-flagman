import { z } from 'zod'
import { procurementItemSchema } from '@/shared/api/recommendation-contracts'
export { procurementItemSchema } from '@/shared/api/recommendation-contracts'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const procurementListSchema = z.object({
  items: z.array(procurementItemSchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100),
  meta: z.object({ server_time: z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }) }).passthrough(),
}).refine(({ items }) => new Set(items.map((item) => item.product_id.toLowerCase())).size === items.length, 'Повторяющиеся продукты')
export type ProcurementItem = z.infer<typeof procurementItemSchema>
export type ProcurementList = z.infer<typeof procurementListSchema>
