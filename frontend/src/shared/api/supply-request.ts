import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'
import { isRestaurantTimezone } from './settings-contracts'

const instant = z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } })
/** Published CreateSupplyRequest: the reception UI uses only explicit expiry. */
export const createSupplyRequestSchema = z.object({
  number: z.string().trim().min(1), received_at: instant,
  items: z.array(z.object({
    product_id: z.uuid(), quantity: z.string().regex(/^\d{1,11}\.\d{3}$/).refine((value) => !/^0+\.000$/.test(value)), expires_at: instant,
  }).strict()).min(1).max(100),
}).strict()
export type CreateSupplyRequest = z.infer<typeof createSupplyRequestSchema>
export const receptionValuesSchema = z.object({
  number: z.string(), receivedAt: z.string(),
  items: z.array(z.object({ productId: z.string(), quantity: z.string(), displayUnit: z.enum(['g', 'kg', 'ml', 'l', 'pcs']), expiresAt: z.string() })).min(1).max(100),
})
export type ReceptionValues = z.infer<typeof receptionValuesSchema>
export const receptionDraftSchema = z.object({
  values: receptionValuesSchema,
  timezone: z.string().refine(isRestaurantTimezone).optional(),
  attempt: z.object({ key: z.uuid(), body: createSupplyRequestSchema, startedAt: z.number().int().nonnegative() }).optional(),
})
export type ReceptionDraft = z.infer<typeof receptionDraftSchema>
