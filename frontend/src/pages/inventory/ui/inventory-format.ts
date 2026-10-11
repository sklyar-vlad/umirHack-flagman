export const productStatuses = { normal: 'Годный остаток', expiring: 'Скоро истекает', expired: 'Есть просрочка', out_of_stock: 'Нет годного остатка' } as const
export const batchStatuses = { valid: 'Годна', expiring: 'Скоро истекает', expired: 'Просрочена', depleted: 'Израсходована' } as const
export function remainingTime(seconds: number | null, expired = false): string {
  if (seconds === null) return 'Нет годных партий'
  const magnitude = Math.abs(seconds)
  const duration = magnitude < 60 ? `${magnitude} с` : magnitude < 3600 ? `${Math.floor(magnitude / 60)} мин` : magnitude < 86400 ? `${Math.floor(magnitude / 3600)} ч` : `${Math.floor(magnitude / 86400)} дн`
  return seconds < 0 || expired ? `Срок истёк ${duration} назад` : `До срока: ${duration}`
}
