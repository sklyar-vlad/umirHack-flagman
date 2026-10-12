import { z } from 'zod'
import { parseApiDateTime } from '@/shared/lib/restaurant-time'

export const userRoleSchema = z.enum(['admin', 'manager', 'storekeeper', 'order_operator'])
const dateTime = z.string().refine((value) => { try { parseApiDateTime(value); return true } catch { return false } })
/** Published AuthUser only. Reject unexpected secrets before the response enters query cache. */
export const userSchema = z.object({
  id: z.uuid(), name: z.string(), email: z.string(), role: userRoleSchema,
  is_active: z.boolean(), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  created_at: dateTime, last_login_at: dateTime.nullable(),
}).strict()
export const userListSchema = z.object({
  items: z.array(userSchema), total: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
  page: z.number().int().positive().max(Number.MAX_SAFE_INTEGER), page_size: z.number().int().min(1).max(100), meta: z.object({}),
}).strict()
export const roleListSchema = z.object({
  items: z.array(z.object({ code: userRoleSchema, name: z.string().min(1) }).strict()).length(4),
}).strict().refine(({ items }) => new Set(items.map((item) => item.code)).size === 4, 'Ожидаются четыре уникальные опубликованные роли')
export type User = z.infer<typeof userSchema>
export type UserList = z.infer<typeof userListSchema>
export type RoleList = z.infer<typeof roleListSchema>
