import type { ForecastRow } from '@/entities/stop-list-forecast'
import { mockId } from './inventory-fixtures'

export const forecastServerTime = '2026-10-13T09:00:00Z'
const common = ['estimate_only', 'no_future_supplies', 'current_recipe_unchanged', 'constant_average_product_consumption_full_lookback', 'continuous_expected_consumption']
// Illustrative server DTOs, not a client implementation of the FEFO forecast model.
export function forecastRows(demo: boolean): ForecastRow[] {
  const rows: ForecastRow[] = [
    { dish_id: mockId(800), dish_name: 'Суп дня', estimated_stop_at: forecastServerTime, cause: 'already_unavailable', limiting_product_id: mockId(10), confidence: 'low', assumptions: common },
    { dish_id: mockId(801), dish_name: 'Салат по срокам партий', estimated_stop_at: '2026-10-13T09:30:00Z', cause: 'expiry', limiting_product_id: mockId(10), confidence: 'insufficient_data', assumptions: [...common, 'expiry_only_for_missing_history', `missing_history:${mockId(10)}`] },
    { dish_id: mockId(802), dish_name: 'Суп по среднему расходу', estimated_stop_at: '2026-10-13T09:45:00.000000001Z', cause: 'estimated_demand', limiting_product_id: mockId(10), confidence: 'low', assumptions: common },
  ]
  if (demo) for (let i = 0; i < 22; i++) rows.push({ dish_id: mockId(4000 + i), dish_name: `Блюдо прогноза ${i + 1}`, estimated_stop_at: '2026-10-13T09:50:00Z', cause: 'estimated_demand', limiting_product_id: mockId(10), confidence: 'low', assumptions: common })
  rows.push(
    { dish_id: mockId(803), dish_name: 'Блюдо без истории расхода', estimated_stop_at: null, cause: 'unknown', limiting_product_id: mockId(10), confidence: 'insufficient_data', assumptions: [...common, 'expiry_only_for_missing_history', `missing_history:${mockId(10)}`, 'unknown_demand_no_expiry_stop_within_horizon'] },
    { dish_id: mockId(804), dish_name: 'Блюдо без остановки в горизонте', estimated_stop_at: null, cause: 'unknown', limiting_product_id: null, confidence: 'low', assumptions: [...common, 'no_stop_within_horizon'] },
  )
  return rows
}
