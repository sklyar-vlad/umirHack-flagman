import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

const instant = z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } }, 'Некорректная дата прогноза')
export const forecastRowSchema = z.object({
  dish_id: z.uuid(), dish_name: z.string(), estimated_stop_at: instant.nullable(),
  cause: z.enum(['already_unavailable', 'expiry', 'estimated_demand', 'unknown']),
  limiting_product_id: z.uuid().nullable(), confidence: z.enum(['low', 'insufficient_data']),
  assumptions: z.array(z.string()),
}).strict()
const integer = z.number().int().safe()
export const forecastReportSchema = z.object({
  items: z.array(forecastRowSchema), total: integer.nonnegative(), page: integer.positive(), page_size: integer.min(1).max(100),
  meta: z.object({ server_time: instant, lookback_days: integer.min(1).max(90), horizon_hours: integer.min(1).max(720) }).passthrough(),
}).strict()
export type ForecastRow = z.infer<typeof forecastRowSchema>
