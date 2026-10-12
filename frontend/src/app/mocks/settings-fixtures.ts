import { settingsSchema, type RestaurantSettings, type UpdateSettings } from '@/shared/api/settings-contracts'

const initial: RestaurantSettings = { restaurant_name: 'Тестовый ресторан', timezone: 'Europe/Moscow', expiring_threshold_hours: 24, auto_writeoff_expired: false, revision: 1 }
const key = 'flagman.mock-settings.v1'
export function mockSettings(): RestaurantSettings {
  try { const saved = settingsSchema.safeParse(JSON.parse(sessionStorage.getItem(key) ?? 'null')); if (saved.success) return saved.data } catch { /* Explicit mock storage can be disabled. */ }
  return { ...initial }
}
export function saveMockSettings(body: UpdateSettings): RestaurantSettings {
  const { expected_revision, ...fields } = body
  const next = { ...mockSettings(), ...fields, revision: expected_revision + 1 }
  sessionStorage.setItem(key, JSON.stringify(next))
  return next
}
export function resetSettingsFixtures() { sessionStorage.removeItem(key) }
