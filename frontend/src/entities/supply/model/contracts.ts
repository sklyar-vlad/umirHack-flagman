import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

const integer = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
export const supplyInstantSchema = z.string().refine((value) => {
  try { parseApiDateTime(value); return true } catch { return false }
}, 'Укажите дату и время с часовым поясом.')
// Published SupplySummary and ListEnvelope in backend/docs/openapi.yaml.
export const supplySummarySchema = z.object({
  id: z.uuid(), number: z.string(), created_at: supplyInstantSchema, received_at: supplyInstantSchema,
  items_count: integer.min(1).max(100), created_by: z.object({ id: z.uuid(), name: z.string() }),
  status: z.literal('received'),
})
export const supplyListSchema = z.object({
  items: z.array(supplySummarySchema), total: integer, page: integer.min(1),
  page_size: integer.min(1).max(100), meta: z.object({}).passthrough(),
})
export type SupplySummary = z.infer<typeof supplySummarySchema>
export type SupplyList = z.infer<typeof supplyListSchema>
