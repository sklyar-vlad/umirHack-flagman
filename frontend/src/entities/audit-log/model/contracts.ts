import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const auditActions = { create: 'Создание', update: 'Изменение', password_change: 'Смена пароля', manual_writeoff: 'Ручное списание', expiration: 'Автоматическое списание' }
export const auditEntities = { product_category: 'Категория', product: 'Продукт', dish: 'Блюдо', supply: 'Поставка', order: 'Заказ', stock_movement: 'Движение', user: 'Пользователь', settings: 'Настройки ресторана', integration_token: 'API-токен', incoming_supply: 'Входящая поставка', hot_offer: 'Горячее предложение' }
export const auditActionSchema = z.enum(['create', 'update', 'password_change', 'manual_writeoff', 'expiration'])
export const auditEntitySchema = z.enum(['product_category', 'product', 'dish', 'supply', 'order', 'stock_movement', 'user', 'settings', 'integration_token', 'incoming_supply', 'hot_offer'])
const text = z.string(), nullableText = text.nullable(), integer = z.number().int().safe(), flag = z.boolean()
const instant = text.refine((value) => { try { parseApiDateTime(value); return true } catch { return false } })
// Mirror the backend's typed snapshot allowlist before entering the query cache.
// Unknown legacy fields, including nested secrets, are discarded, never rendered as raw JSON.
const workflow = z.object({ name: text, status: text, revision: integer, is_published: flag, is_manual: flag, message: text, external_id: text, items: z.array(z.object({ id: text, product_id: text, quantity: text, accepted_quantity: text })) }).partial()
const snapshots = {
 integration_token: workflow, incoming_supply: workflow, hot_offer: workflow,
  product_category: z.object({ name: text, revision: integer }).partial(),
  product: z.object({ name: text, category_id: text, unit: text, display_unit: text, standard_shelf_life_hours: integer.nullable(), target_stock: nullableText, is_active: flag, revision: integer }).partial(),
  dish: z.object({ name: text, description: nullableText, is_active: flag, revision: integer, recipe_version_id: nullableText, recipe_version: integer.nullable(), recipe: z.array(z.object({ product_id: text, quantity_per_portion: text })).nullable() }).partial(),
  supply: z.object({ number: text, received_at: text, batches: z.array(z.object({ batch_id: text, product_id: text, line_number: integer, initial_quantity: text, remaining_quantity: text, produced_at: nullableText, shelf_life_hours: integer.nullable(), expiry_source: text, expires_at: text })).nullable() }).partial(),
  order: z.object({ number: integer, items: z.array(z.object({ dish_id: text, recipe_version_id: text, dish_name_snapshot: text, portions: integer })).nullable() }).partial(),
  stock_movement: z.object({ batch_id: text, order_item_id: nullableText, type: text, delta_quantity: text, remaining_quantity_after: text, product_name_snapshot: text, unit_snapshot: text, note: nullableText }).partial(),
  user: z.object({ name: text, email: text, role: text, is_active: flag, revision: integer }).partial(),
  settings: z.object({ restaurant_name: text, timezone: text, expiring_threshold_hours: integer, auto_writeoff_expired: flag, revision: integer }).partial(),
}
const entry = z.object({ id: z.uuid(), created_at: instant, actor: z.object({ id: z.uuid(), name: text }).strict().nullable(), action: auditActionSchema, entity_type: auditEntitySchema, entity_id: z.uuid().nullable(), request_id: nullableText, before: z.unknown(), after: z.unknown() }).strict()
export const auditEntrySchema = entry.transform((value, ctx) => {
  const schema = snapshots[value.entity_type].nullable()
  const before = schema.safeParse(value.before), after = schema.safeParse(value.after)
  if (!before.success || !after.success) { ctx.addIssue({ code: 'custom', message: 'Некорректные изменения журнала' }); return z.NEVER }
  return { ...value, before: before.data, after: after.data }
})
export const auditListSchema = z.object({ items: z.array(auditEntrySchema), total: integer.nonnegative(), page: integer.positive(), page_size: integer.min(1).max(100), meta: z.object({}) }).strict()
export type AuditEntry = z.infer<typeof auditEntrySchema>
export type AuditEntity = z.infer<typeof auditEntitySchema>
