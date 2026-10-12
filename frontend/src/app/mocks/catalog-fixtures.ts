import type { Product } from '@/entities/product'
import type { Category } from '@/entities/product-category'
import { inventoryCategories, inventoryItems } from './inventory-fixtures'
import type { InventoryItem } from '@/entities/inventory'
import { parseQuantity, serializeQuantity } from '@/shared/lib/quantity'
let categories: Category[] = structuredClone(inventoryCategories)
let products: Product[] = []
const productStorageKey = 'flagman.mock.products.v1'
const storageKey = 'flagman.mock.categories.v1'
export function catalogCategories() {
  try { categories = JSON.parse(sessionStorage.getItem(storageKey) ?? JSON.stringify(categories)) as Category[] } catch { /* Node tests use memory. */ }
  return categories
}
export function saveCatalogCategory(category: Category) {
  const items = catalogCategories(), index = items.findIndex((item) => item.id === category.id)
  categories = index < 0 ? [...items, category] : items.map((item) => item.id === category.id ? category : item)
  try { sessionStorage.setItem(storageKey, JSON.stringify(categories)) } catch { /* Node tests. */ }
}
export function resetCatalogFixtures() { categories = structuredClone(inventoryCategories); products = []; try { sessionStorage.removeItem(productStorageKey); sessionStorage.removeItem(storageKey) } catch { /* Node tests. */ } }
export function catalogProducts() {
  const disabled = inventoryItems.filter((item) => !item.product.is_active).map(({ product }) => ({ ...product, standard_shelf_life_hours: null, target_stock: null, revision: 1, created_at: '2026-01-01T00:00:00Z' }))
  try { products = JSON.parse(sessionStorage.getItem(productStorageKey) ?? JSON.stringify(products)) as Product[] } catch { /* Node tests. */ }
  const active = inventoryItems.filter((item) => item.product.is_active).map(({ product }) => ({ ...product, standard_shelf_life_hours: 48, target_stock: null, revision: 1, created_at: '2026-01-01T00:00:00Z' }))
  const originals = [...active, ...disabled]
  return [...originals.map((product) => products.find((saved) => saved.id === product.id) ?? product), ...products.filter((saved) => !originals.some((product) => product.id === saved.id))].map((product) => ({ ...product,
    category: catalogCategories().find((category) => category.id === product.category.id) ?? product.category,
    target_stock: !products.some((saved) => saved.id === product.id) && product.name === 'Сахар' ? '0.000' : !products.some((saved) => saved.id === product.id) && product.name === 'Молоко' ? '4000.000' : product.target_stock,
  }))
}

export function saveCatalogProduct(product: Product) {
  catalogProducts()
  products = [...products.filter((saved) => saved.id !== product.id), product]
  try { sessionStorage.setItem(productStorageKey, JSON.stringify(products)) } catch { /* Node tests. */ }
}

export function changedCatalogProducts() { catalogProducts(); return products }
/** Synthetic read projection: old batch quantities/expiry snapshots are never changed. */
export function catalogInventoryItems(): InventoryItem[] {
  const changed = changedCatalogProducts()
  const newItems = changed.filter((product) => !inventoryItems.some((item) => item.product.id === product.id)).map((product) => ({ ...inventoryItems[0], product, physical_quantity: '0.000', available_quantity: '0.000', expired_quantity: '0.000', expiring_quantity: '0.000', target_stock: product.target_stock, deficit_quantity: product.target_stock, batches_count: 0, nearest_expires_at: null, remaining_seconds: null, has_expiring: false, has_expired: false, is_out_of_stock: true, status: 'out_of_stock' as const }))
  return [...inventoryItems, ...newItems].map((item) => {
    const product = changed.find((product) => product.id === item.product.id)
    if (!product) return item
    const deficit = product.target_stock === null ? null : parseQuantity(product.target_stock)! - parseQuantity(item.available_quantity)!
    return { ...item, product, target_stock: product.target_stock, deficit_quantity: deficit === null ? null : serializeQuantity(deficit > 0n ? deficit : 0n) }
  })
}
