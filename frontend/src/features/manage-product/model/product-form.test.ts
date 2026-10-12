import { describe, expect, it } from 'vitest'
import { createProductSchema, updateProductSchema } from '@/entities/product'
import { emptyProduct, productFields, productFormSchema, productPatch, productToValues } from './product-form'
import { catalogProducts } from '@/app/mocks/catalog-fixtures'
import { mockId } from '@/app/mocks/inventory-fixtures'
const valid = { ...emptyProduct, name: '  Новый продукт  ', category_id: mockId(2), target_stock: '99999999999,999', standard_shelf_life_hours: '2147483647' }
describe('product validation and exact published payloads', () => {
  it('trims Unicode names and keeps max numeric(14,3) without floating point', () => {
    expect(productFormSchema.safeParse(valid).success).toBe(true)
    expect(productFields(valid)).toMatchObject({ name: 'Новый продукт', target_stock: '99999999999.999', standard_shelf_life_hours: 2147483647 })
    expect(productFormSchema.safeParse({ ...valid, name: '😀'.repeat(200) }).success).toBe(true)
    expect(productFormSchema.safeParse({ ...valid, name: '😀'.repeat(201) }).success).toBe(false)
  })
  for (const [field, value] of [['name', '   '], ['category_id', 'invalid'], ['standard_shelf_life_hours', '0'], ['standard_shelf_life_hours', '1.5'], ['standard_shelf_life_hours', '2147483648'], ['target_stock', '100000000000'], ['target_stock', '-1'], ['target_stock', '1e2'], ['target_stock', '0.0001']] as const) it(`rejects ${field}=${value}`, () => {
    const result = productFormSchema.safeParse({ ...valid, [field]: value })
    expect(result.success).toBe(false); if (!result.success) expect(result.error.issues.some((issue) => issue.path[0] === field)).toBe(true)
  })
  it('validates unit compatibility and integer pieces', () => {
    expect(productFormSchema.safeParse({ ...valid, display_unit: 'l' }).success).toBe(false)
    expect(productFormSchema.safeParse({ ...valid, unit: 'pcs', display_unit: 'pcs', target_stock: '1.001' }).success).toBe(false)
    expect(productFields({ ...valid, unit: 'pcs', display_unit: 'pcs', target_stock: '2.000' }).target_stock).toBe('2.000')
    expect(productFields({ ...valid, display_unit: 'kg', target_stock: '1,001' }).target_stock).toBe('1.001')
  })
  it('distinguishes no change, explicit null and zero; omits unrelated PATCH fields', () => {
    const values = productToValues(catalogProducts().find((product) => product.name === 'Молоко')!)
    const baseline = { fields: productFields(values), revision: 1 }
    expect(productPatch(values, baseline)).toEqual({ expected_revision: 1 })
    expect(productPatch({ ...values, name: 'Новое имя' }, baseline)).toEqual({ expected_revision: 1, name: 'Новое имя' })
    expect(productPatch({ ...values, target_stock: '', standard_shelf_life_hours: '' }, baseline)).toEqual({ expected_revision: 1, target_stock: null, standard_shelf_life_hours: null })
    expect(productPatch({ ...values, target_stock: '0' }, baseline)).toEqual({ expected_revision: 1, target_stock: '0.000' })
  })
  it('rejects invalid published requests and no-op PATCH; permits omission/defaults', () => {
    expect(createProductSchema.safeParse({ name: 'Имя', category_id: mockId(2), unit: 'g', display_unit: 'kg' }).success).toBe(true)
    expect(createProductSchema.safeParse({ ...productFields(valid), extra: true }).success).toBe(false)
    expect(updateProductSchema.safeParse({ expected_revision: 1 }).success).toBe(false)
    expect(updateProductSchema.safeParse({ expected_revision: 2147483647, target_stock: null }).success).toBe(false)
    expect(updateProductSchema.safeParse({ expected_revision: 1, category_id: null }).success).toBe(false)
    expect(updateProductSchema.safeParse({ expected_revision: 1, target_stock: null }).success).toBe(true)
  })
})
