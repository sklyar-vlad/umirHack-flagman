import { z } from 'zod'

export function isRestaurantTimezone(value: string) {
  if (!value || value === 'Local' || Array.from(value).length > 100) return false
  try { new Intl.DateTimeFormat('ru', { timeZone: value }); return true } catch { return false }
}
const name = z.string().trim().refine((value) => Array.from(value).length >= 1 && Array.from(value).length <= 200, 'Название: от 1 до 200 символов.')
const timezone = z.string().refine(isRestaurantTimezone, 'Укажите IANA timezone, например Europe/Moscow. Local запрещён.')
const threshold = z.number().int().min(1).max(720)
export const settingsSchema = z.strictObject({
  restaurant_name: name, timezone, expiring_threshold_hours: threshold,
  auto_writeoff_expired: z.boolean(), revision: z.number().int().min(1).max(2147483647),
})
export const updateSettingsSchema = z.strictObject({
  restaurant_name: name.optional(), timezone: timezone.optional(), expiring_threshold_hours: threshold.optional(),
  auto_writeoff_expired: z.boolean().optional(), expected_revision: z.number().int().min(1).max(2147483646),
}).refine((value) => Object.keys(value).some((key) => key !== 'expected_revision'), 'Измените хотя бы одну настройку.')
export type RestaurantSettings = z.infer<typeof settingsSchema>
export type UpdateSettings = z.infer<typeof updateSettingsSchema>
