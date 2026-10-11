import { z } from 'zod'
import { dishSchema } from './contracts'
import { hotOfferSchema } from '@/shared/api/recommendation-contracts'
export { hotOfferSchema, hotOfferBatchSchema } from '@/shared/api/recommendation-contracts'

const integer = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const hotOffersListSchema = z.object({
  items: z.array(hotOfferSchema), total: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
  page: integer, page_size: integer.max(100), meta: dishSchema.shape.meta,
}).refine(({ items }) => new Set(items.map((item) => item.dish.id.toLowerCase())).size === items.length, 'Повторяющееся предложение.')
export type HotOffer = z.infer<typeof hotOfferSchema>
export type HotOffersList = z.infer<typeof hotOffersListSchema>
