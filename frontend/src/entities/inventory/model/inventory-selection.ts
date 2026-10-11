import { z } from 'zod'

/** UI selection only; these identifiers are never inventory API filter parameters. */
export function readInventorySelection(params: URLSearchParams) {
  const product = params.get('product_id'), supply = params.get('supply_id')
  return {
    productId: z.uuid().safeParse(product).success ? product! : '',
    supplyId: z.uuid().safeParse(supply).success ? supply! : '',
  }
}
export function inventorySelectionParams(params: URLSearchParams): URLSearchParams {
  const selection = readInventorySelection(params), result = new URLSearchParams()
  if (selection.productId) result.set('product_id', selection.productId)
  if (selection.productId && selection.supplyId) result.set('supply_id', selection.supplyId)
  return result
}
export function supplyInventoryUrl(product: { id: string; name: string }, supplyId: string) {
  return `/inventory?${new URLSearchParams({ search: product.name, product_id: product.id, supply_id: supplyId })}`
}
