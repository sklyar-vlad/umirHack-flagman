import type { AuditEntry } from './contracts'

export function auditObjectLabel(entry: AuditEntry): string {
  for (const snapshot of [entry.after, entry.before]) {
    if (!snapshot) continue
    for (const field of ['name', 'product_name_snapshot', 'restaurant_name'] as const) {
      if (field in snapshot) {
        const value = (snapshot as Record<string, unknown>)[field]
        if (typeof value === 'string' && value) return value
      }
    }
    if ('number' in snapshot && snapshot.number !== undefined) return `№${snapshot.number}`
  }
  return entry.entity_id ?? (entry.entity_type === 'settings' ? 'Настройки ресторана' : 'ID не указан')
}
