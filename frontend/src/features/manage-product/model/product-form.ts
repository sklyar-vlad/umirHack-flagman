import { productValuesSchema, type ProductValues, type ProductFields } from '@/shared/api/product-work'
import { quantityFromInput } from '@/shared/lib/quantity'
import type { Product, UpdateProduct } from '@/entities/product'

export const emptyProduct: ProductValues = { name: '', category_id: '', unit: 'g', display_unit: 'g', standard_shelf_life_hours: '', target_stock: '', is_active: true }
export const productFormSchema = productValuesSchema.superRefine((value, ctx) => {
  if ([...value.name.trim()].length < 1 || [...value.name.trim()].length > 200) ctx.addIssue({ code: 'custom', path: ['name'], message: 'Название: от 1 до 200 символов после удаления пробелов' })
  if (!/^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/i.test(value.category_id)) ctx.addIssue({ code: 'custom', path: ['category_id'], message: 'Выберите категорию' })
  if (value.unit !== value.display_unit && !(value.unit === 'g' && value.display_unit === 'kg') && !(value.unit === 'ml' && value.display_unit === 'l')) ctx.addIssue({ code: 'custom', path: ['display_unit'], message: 'Единица отображения несовместима с базовой' })
  if (value.standard_shelf_life_hours.trim() && (!/^\d+$/.test(value.standard_shelf_life_hours.trim()) || Number(value.standard_shelf_life_hours) < 1 || Number(value.standard_shelf_life_hours) > 2147483647)) ctx.addIssue({ code: 'custom', path: ['standard_shelf_life_hours'], message: 'Срок: целое число от 1 до 2147483647 часов или пустое поле' })
  try { if (value.target_stock.trim()) quantityFromInput(value.target_stock, value.unit, value.unit, { allowZero: true }) } catch (error) { ctx.addIssue({ code: 'custom', path: ['target_stock'], message: error instanceof Error ? error.message : 'Проверьте целевой запас' }) }
})
export function productFields(values: ProductValues): ProductFields {
  return { name: values.name.trim(), category_id: values.category_id, unit: values.unit, display_unit: values.display_unit,
    standard_shelf_life_hours: values.standard_shelf_life_hours.trim() ? Number(values.standard_shelf_life_hours) : null,
    target_stock: values.target_stock.trim() ? quantityFromInput(values.target_stock, values.unit, values.unit, { allowZero: true }) : null, is_active: values.is_active }
}
export function productToValues(product: Product): ProductValues {
  return { name: product.name, category_id: product.category.id, unit: product.unit, display_unit: product.display_unit,
    standard_shelf_life_hours: product.standard_shelf_life_hours?.toString() ?? '', target_stock: product.target_stock ?? '', is_active: product.is_active }
}
export function productPatch(values: ProductValues, baseline: { fields: ProductFields; revision: number }): UpdateProduct {
  const fields = productFields(values), changed = Object.fromEntries(Object.entries(fields).filter(([key, value]) => value !== baseline.fields[key as keyof ProductFields]))
  return { ...changed, expected_revision: baseline.revision }
}
