/** Clock notifications only need REST when a displayed batch crosses a boundary. */
export function expiryBoundaryCrossed(data: unknown, now: number, since: number, thresholdHours: number): boolean {
  if (!data || typeof data !== 'object') return false
  if (Array.isArray(data)) return data.some((item) => expiryBoundaryCrossed(item, now, since, thresholdHours))
  const record = data as Record<string, unknown>
  const expires = record.expires_at ?? record.nearest_expires_at
  if (typeof expires === 'string' && typeof record.remaining_seconds === 'number' && record.remaining_seconds > 0) {
    const end = Date.parse(expires), snapshot = end - record.remaining_seconds * 1000
    if ([end, end - thresholdHours * 3_600_000].some((boundary) => boundary > Math.max(snapshot, since) && boundary <= now)) return true
  }
  return Object.values(record).some((value) => typeof value === 'object' && expiryBoundaryCrossed(value, now, since, thresholdHours))
}
