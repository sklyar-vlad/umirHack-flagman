import { z } from 'zod'

export const eventTypes = ['supplies.incoming_changed', 'inventory.changed', 'inventory.time_changed', 'products.changed', 'product_categories.changed', 'dishes.changed', 'settings.changed', 'users.changed', 'resync.required'] as const
export const eventTopics = ['inventory', 'batches', 'dashboard', 'dishes', 'stop-list', 'hot-offers', 'procurement-recommendations', 'products', 'product-categories', 'settings', 'users', 'stock-movements', 'supplies', 'orders'] as const
export type EventTopic = typeof eventTopics[number]
const ids = z.array(z.string().uuid()).max(10_000).optional()
const payloadSchema = z.object({
  occurred_at: z.iso.datetime({ offset: true }), topics: z.array(z.enum(eventTopics)),
  product_ids: ids, batch_ids: ids, dish_ids: ids, category_ids: ids, supply_ids: ids, order_ids: ids, user_ids: ids,
})

export function decodeEvent(type: string, cursor: string, data: string) {
  if (!eventTypes.some((value) => value === type) || !/^\d{1,19}$/.test(cursor) || BigInt(cursor) > 9223372036854775807n || (BigInt(cursor) === 0n && type !== 'resync.required') || data.length > 1_000_000) return null
  try {
    const result = payloadSchema.safeParse(JSON.parse(data))
    return result.success ? { type, cursor: BigInt(cursor), ...result.data } : null
  } catch { return null }
}
export type LiveEvent = NonNullable<ReturnType<typeof decodeEvent>>
