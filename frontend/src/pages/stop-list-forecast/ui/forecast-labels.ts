import type { ForecastRow } from '@/entities/stop-list-forecast'

export const causeLabels: Record<ForecastRow['cause'], string> = {
  already_unavailable: 'Уже недоступно', expiry: 'Истечение срока партии', estimated_demand: 'Ожидаемый расход', unknown: 'Причина остановки не определена',
}
export function forecastOutcome(row: ForecastRow): string {
  if (row.estimated_stop_at !== null) return row.cause === 'already_unavailable' ? 'Недоступно на момент снимка' : 'Оценка времени остановки'
  if (row.assumptions.includes('unknown_demand_no_expiry_stop_within_horizon')) return 'Недостаточно истории; по срокам остановки в горизонте не найдено'
  if (row.assumptions.includes('no_stop_within_horizon')) return 'Остановка в пределах горизонта не ожидается'
  return row.confidence === 'insufficient_data' ? 'Недостаточно истории для оценки спроса' : 'Время остановки не определено'
}
const assumptions: Record<string, string> = {
  estimate_only: 'Это оценка, а не гарантия', no_future_supplies: 'Будущие поставки не учитываются', current_recipe_unchanged: 'Текущая рецептура считается неизменной',
  constant_average_product_consumption_full_lookback: 'Средний расход продуктов постоянен и рассчитан за все дни выбранного периода',
  continuous_expected_consumption: 'Ожидаемый расход моделируется непрерывно', expiry_only_for_missing_history: 'Для продуктов без истории оцениваются только сроки партий, без неизвестного расхода',
  recipe_missing: 'У блюда нет рецептуры', no_stop_within_horizon: 'Модель не нашла остановки в пределах выбранного горизонта',
  unknown_demand_no_expiry_stop_within_horizon: 'Спрос неизвестен; только по срокам остановки в пределах горизонта не найдено',
}
export function assumptionLabel(value: string): string {
  return assumptions[value] ?? (value.startsWith('missing_history:') ? `Нет истории расхода в текущей единице продукта ${value.slice('missing_history:'.length)}` : value)
}
