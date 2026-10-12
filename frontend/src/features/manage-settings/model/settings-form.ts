import { z } from 'zod'
import { settingsSchema, updateSettingsSchema, type RestaurantSettings, type UpdateSettings } from '@/shared/api/settings-contracts'

export const settingsFormSchema = settingsSchema.omit({ revision: true }).extend({
  expiring_threshold_hours: z.string().regex(/^\d+$/, 'Укажите целое число от 1 до 720.').refine((value) => Number(value) >= 1 && Number(value) <= 720, 'Порог: от 1 до 720 часов.'),
})
export type SettingsValues = z.infer<typeof settingsFormSchema>
export function settingsValues(settings: RestaurantSettings): SettingsValues { return { restaurant_name: settings.restaurant_name, timezone: settings.timezone, expiring_threshold_hours: String(settings.expiring_threshold_hours), auto_writeoff_expired: settings.auto_writeoff_expired } }
export function settingsPatch(values: SettingsValues, original: RestaurantSettings): UpdateSettings | null {
  const parsed = settingsFormSchema.parse(values)
  const next = { ...parsed, expiring_threshold_hours: Number(parsed.expiring_threshold_hours) }
  const changes = Object.fromEntries(Object.entries(next).filter(([field, value]) => original[field as keyof RestaurantSettings] !== value))
  return Object.keys(changes).length ? updateSettingsSchema.parse({ ...changes, expected_revision: original.revision }) : null
}
export function settingsField(path: string): keyof SettingsValues | null {
  return ['restaurant_name', 'timezone', 'expiring_threshold_hours', 'auto_writeoff_expired'].includes(path) ? path as keyof SettingsValues : null
}
