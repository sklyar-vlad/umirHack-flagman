import { describe, expect, it } from 'vitest'
import { compositionDishes } from '@/app/mocks/order-composition-fixtures'
import { orderRequestSchema, orderDraftSchema } from '@/shared/api/order-work'
import { addBasketDish, basketRequest, basketFingerprint, canRetryOrder, recipeChanged, uncertainOrder } from './composition'
const dish = compositionDishes()[0]
describe('basket limits and accepted recipe identity', () => {
  it('merges catalogue and recommendation additions by dish_id into the same basket', () => {
    const first = addBasketDish({ lines: [] }, dish, 999)
    const combined = addBasketDish(first, dish)
    expect(combined.lines).toHaveLength(1)
    expect(basketRequest(combined)).toEqual({ items: [{ dish_id: dish.id, portions: 1000, recipe_version: 1 }] })
    expect(() => addBasketDish(combined, dish)).toThrow()
  })
  it.each(['', '0', '-1', '1.5', '1e2', ' 2', '1001', 'Infinity'])('rejects invalid portions %s', (portions) => expect(basketRequest({ lines: [{ dish, portions }] })).toBeNull())
  it('enforces 1–100 lines, merged totals and conflicting repeated versions', () => {
    expect(orderRequestSchema.safeParse({ items: [] }).success).toBe(false)
    const line = { dish_id: dish.id, portions: 500, recipe_version: 1 }
    expect(orderRequestSchema.safeParse({ items: [line, line] }).success).toBe(true)
    expect(orderRequestSchema.safeParse({ items: [line, { ...line, portions: 501 }] }).success).toBe(false)
    expect(orderRequestSchema.safeParse({ items: [line, { ...line, recipe_version: 2 }] }).success).toBe(false)
    expect(orderRequestSchema.safeParse({ items: Array.from({ length: 101 }, () => ({ ...line, portions: 1 })) }).success).toBe(false)
    const full = { lines: Array.from({ length: 100 }, (_, i) => ({ dish: { ...dish, id: `00000000-0000-4000-8000-${String(i).padStart(12, '0')}` }, portions: '1' })) }
    expect(() => addBasketDish(full, dish)).toThrow()
    expect(orderDraftSchema.safeParse({ lines: [{ dish, portions: '1' }, { dish, portions: '2' }] }).success).toBe(false)
  })
  it('locks uncertain attempts; fingerprints include versions/compositions and require explicit recipe acceptance', () => {
    const draft = addBasketDish({ lines: [] }, dish)
    const changed = compositionDishes(true)[0]
    expect(recipeChanged(dish, changed)).toBe(true)
    expect(basketFingerprint(draft)).not.toBe(basketFingerprint({ lines: [{ dish: changed, portions: '1' }] }))
    expect(() => addBasketDish(draft, changed)).toThrow()
    expect(() => addBasketDish({ ...draft, attempt: { key: crypto.randomUUID(), body: basketRequest(draft)!, startedAt: Date.now() } }, dish)).toThrow()
    expect(basketRequest({ lines: [{ dish: compositionDishes(false, true)[0], portions: '1' }] })).toBeNull()
  })
  it('preserves exact attempts on transport/access/rate failures, limits replay lifetime', () => {
    for (const status of ['NETWORK_ERROR', 'INVALID_RESPONSE', 500, 401, 403, 429] as const) expect(uncertainOrder({ status, code: 'ERROR', message: '', details: {} })).toBe(true)
    for (const status of [409, 422]) expect(uncertainOrder({ status, code: 'INSUFFICIENT_STOCK', message: '', details: {} })).toBe(false)
    expect(canRetryOrder(100, 99)).toBe(false)
    expect(canRetryOrder(100, 100 + 23 * 3600000)).toBe(false)
  })
})
