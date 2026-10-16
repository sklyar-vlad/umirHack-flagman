import { z } from 'zod'
import { basketDishSchema, orderRequestSchema, type BasketDish, type OrderDraft, type OrderRequest } from '@/shared/api/order-work'
import type { ApiError } from '@/shared/api'

export const portionsSchema = z.string().regex(/^\d+$/, 'Введите целое число порций.').refine((value) => Number(value) >= 1 && Number(value) <= 1000, 'От 1 до 1000 порций одного блюда.')
export function addBasketDish(draft: OrderDraft, dish: BasketDish, portions = 1): OrderDraft {
  portionsSchema.parse(String(portions))
  if (draft.attempt) throw new Error('Сначала выясните результат исходного запроса.')
  if (!dish.is_active || !dish.recipe_version || !dish.recipe.length) throw new Error('Блюдо отключено или не имеет рецептуры.')
  const prior = draft.lines.find((line) => line.dish.id === dish.id)
  if (prior && recipeChanged(prior.dish, dish)) throw new Error('Подтвердите актуальную рецептуру в корзине.')
  const next = portionsSchema.parse(String((prior ? Number(portionsSchema.parse(prior.portions)) : 0) + portions))
  if (!prior && draft.lines.length >= 100) throw new Error('В корзине может быть не больше 100 блюд.')
  return { lines: prior ? draft.lines.map((line) => line.dish.id === dish.id ? { ...line, portions: next } : line) : [...draft.lines, { dish: basketDishSchema.parse(dish), portions: next }] }
}
export function basketRequest(draft: OrderDraft): OrderRequest | null {
  if (!draft.lines.length || draft.lines.some((line) => !line.dish.is_active || !line.dish.recipe_version || !line.dish.recipe.length || !portionsSchema.safeParse(line.portions).success)) return null
  const parsed = orderRequestSchema.safeParse({ items: draft.lines.map(({ dish, portions }) => ({ dish_id: dish.id, portions: Number(portions), recipe_version: dish.recipe_version })) })
  return parsed.success ? parsed.data : null
}
export const basketFingerprint = (draft: OrderDraft) => JSON.stringify(draft.lines)
export const recipeChanged = (accepted: BasketDish, current: BasketDish) => accepted.recipe_version !== current.recipe_version || accepted.is_active !== current.is_active || JSON.stringify(accepted.recipe) !== JSON.stringify(current.recipe)
export function uncertainOrder(error: ApiError) {
  return typeof error.status !== 'number' || error.status >= 500 || [401, 403, 408, 429].includes(error.status) || ['IDEMPOTENCY_CONFLICT', 'REQUEST_IN_PROGRESS', 'SESSION_CHANGED'].includes(error.code)
}
export const canRetryOrder = (startedAt: number, now = Date.now()) => now >= startedAt && now - startedAt < 23 * 60 * 60 * 1000
