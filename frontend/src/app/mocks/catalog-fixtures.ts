import type { Category } from '@/entities/product-category'
import { inventoryCategories, inventoryItems } from './inventory-fixtures'
import { receptionProducts } from './reception-fixtures'
let categories: Category[] = structuredClone(inventoryCategories)
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
export function resetCatalogFixtures() { categories = structuredClone(inventoryCategories); try { sessionStorage.removeItem(storageKey) } catch { /* Node tests. */ } }
export function catalogProducts() {
  const disabled = inventoryItems.filter((item) => !item.product.is_active).map(({ product }) => ({ ...product, standard_shelf_life_hours: null, target_stock: null, revision: 1, created_at: '2026-01-01T00:00:00Z' }))
  return [...receptionProducts, ...disabled].map((product) => ({ ...product,
    category: catalogCategories().find((category) => category.id === product.category.id) ?? product.category,
    target_stock: product.name === 'Сахар' ? '0.000' : product.name === 'Молоко' ? '4000.000' : product.target_stock,
  }))
}
