import { describe, expect, it } from 'vitest'
import { settingsSchema, updateSettingsSchema } from '@/shared/api/settings-contracts'
import { settingsFormSchema, settingsPatch, settingsValues } from './settings-form'
const initial = { restaurant_name: 'Ресторан', timezone: 'Europe/Moscow', expiring_threshold_hours: 24, auto_writeoff_expired: false, revision: 7 }
describe('published restaurant settings', () => {
  it('builds a minimal PATCH, retaining false and exact revision', () => {
    expect(settingsPatch({ ...settingsValues(initial), restaurant_name: '  Кухня  ', expiring_threshold_hours: '1' }, initial)).toEqual({ restaurant_name: 'Кухня', expiring_threshold_hours: 1, expected_revision: 7 })
    expect(settingsPatch(settingsValues(initial), initial)).toBeNull()
    expect(settingsPatch(settingsValues(initial), { ...initial, auto_writeoff_expired: true })).toEqual({ expected_revision: 7, auto_writeoff_expired: false })
  })
  it.each(['0', '721', '-1', '1.5', '', 'NaN', '1e2', ' 24'])('rejects invalid threshold %s before transport', (value) => {
    expect(settingsFormSchema.safeParse({ ...settingsValues(initial), expiring_threshold_hours: value }).success).toBe(false)
  })
  it.each(['Local', 'Mars/Olympus', '', ' Europe/Moscow'])('rejects invalid zone %s', (timezone) => {
    expect(settingsSchema.safeParse({ ...initial, timezone }).success).toBe(false)
  })
  it.each(['UTC', 'Europe/Moscow', 'Asia/Yekaterinburg', 'America/New_York'])('accepts explicit supported zone %s', (timezone) => {
    expect(settingsSchema.safeParse({ ...initial, timezone }).success).toBe(true)
  })
  it('counts Unicode characters, rejects null/unknown fields and empty PATCH', () => {
    expect(settingsSchema.safeParse({ ...initial, restaurant_name: '😀'.repeat(200) }).success).toBe(true)
    expect(settingsSchema.safeParse({ ...initial, restaurant_name: '😀'.repeat(201) }).success).toBe(false)
    expect(updateSettingsSchema.safeParse({ expected_revision: 1 }).success).toBe(false)
    expect(updateSettingsSchema.safeParse({ expected_revision: 1, timezone: null }).success).toBe(false)
    expect(updateSettingsSchema.safeParse({ expected_revision: 1, worker_ready: true }).success).toBe(false)
    expect(updateSettingsSchema.safeParse({ expected_revision: 2147483647, timezone: 'UTC' }).success).toBe(false)
  })
})
