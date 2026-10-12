import { z } from 'zod'
import { batchSchema, productBriefSchema } from './warehouse-contracts'

/** Published WriteOffRequest; quantities are in the batch's base unit. */
export const writeOffRequestSchema = z.object({
  batch_id: z.uuid(),
  quantity: z.string().regex(/^\d{1,11}(?:\.\d{1,3})?$/).refine((value) => !/^0+(?:\.0+)?$/.test(value)),
  reason: z.string().trim().refine((value) => [...value].length >= 1 && [...value].length <= 500 && !/[\p{Cc}]/u.test(value)),
}).strict()
export type WriteOffRequest = z.infer<typeof writeOffRequestSchema>
export const writeOffValuesSchema = z.object({ quantity: z.string(), reason: z.string() })
export type WriteOffValues = z.infer<typeof writeOffValuesSchema>
export const writeOffDraftSchema = z.object({
  batch: batchSchema, product: productBriefSchema, values: writeOffValuesSchema,
  attempt: z.object({ key: z.uuid(), body: writeOffRequestSchema, startedAt: z.number().int().nonnegative() }).optional(),
}).refine((value) => value.batch.product_id === value.product.id && value.batch.unit === value.product.unit &&
  (!value.attempt || value.attempt.body.batch_id === value.batch.id && (value.batch.unit !== 'pcs' || /^\d+(?:\.0+)?$/.test(value.attempt.body.quantity))))
export type WriteOffDraft = z.infer<typeof writeOffDraftSchema>
