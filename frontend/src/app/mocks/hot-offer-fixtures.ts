import { applyBatchWriteOff } from './write-off-fixtures'
import type { HotOffer } from '@/entities/dish'
import { compositionDishes, consumedCompositionAmounts } from './order-composition-fixtures'
import { inventoryBatches, inventoryItems } from './inventory-fixtures'
import { getMockScenario } from './scenarios'
import { mockSettings } from './settings-fixtures'
let expiry: number | undefined
export function resetHotOfferFixtures() { expiry = undefined }
export function hotOfferItems(): HotOffer[] {
  const scenario = getMockScenario(), now = Date.now()
  if (scenario === 'hot-offers-empty' || scenario === 'hot-offers-expired') return []
  if (scenario === 'hot-offers-expiring') { expiry ??= now + 1000; if (now >= expiry) return [] }
  const batch = applyBatchWriteOff(inventoryBatches[1]), remaining = Number(batch.remaining_quantity) - (consumedCompositionAmounts()[batch.id] ?? 0)
  if (remaining <= 0) return []
  return compositionDishes(scenario === 'order-recipe-changed', scenario === 'order-inactive').filter((dish) => dish.is_active && dish.is_available && dish.recipe.some((line) => line.product_id === batch.product_id)).map((dish, i): HotOffer => {
    const seconds = scenario === 'hot-offers-expiring' ? Math.max(0, Math.floor((expiry! - now) / 1000)) : scenario === 'hot-offers-high' && i === 0 ? 3600 : i === 0 ? 64800 : 36000
    const expires = scenario === 'hot-offers-expiring' ? new Date(expiry!).toISOString() : new Date(now + seconds * 1000).toISOString()
    const used = Math.min(Number(dish.recipe.find((line) => line.product_id === batch.product_id)!.quantity_per_portion), remaining).toFixed(3)
    return { dish: { id: dish.id, name: dish.name, recipe_version: dish.recipe_version! }, rank: i + 1,
      priority_score: scenario === 'hot-offers-high' && i === 0 ? '95.0000' : '1.6667', priority_level: scenario === 'hot-offers-high' && i === 0 ? 'high' : i === 0 ? 'low' : 'medium',
      available_portions: dish.available_portions, reason: `Одна порция использует ${used} мл молока из истекающей партии поставки №${batch.supply.number}.`,
      problem_batches: [{ batch_id: batch.id, product_id: batch.product_id, product_name: inventoryItems[0].product.name, supply_id: batch.supply.id, supply_number: batch.supply.number,
        line_number: batch.line_number, unit: batch.unit, remaining_quantity: remaining.toFixed(3), used_for_one_portion: used, expires_at: expires, remaining_seconds: seconds }],
    }
  }).filter((offer) => offer.problem_batches.every((batch) => batch.remaining_seconds <= mockSettings().expiring_threshold_hours * 3600)).map((offer, i) => ({ ...offer, rank: i + 1 }))
}
