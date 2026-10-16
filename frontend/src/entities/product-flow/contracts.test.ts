import { describe,expect,it } from 'vitest'
import { flowRowSchema } from './index'
const row = { product_id:'00000000-0000-4000-8000-000000000001',product_name:'Молоко',unit:'ml',received_quantity:'100.000',ordered_quantity:'20.000',expired_quantity:'2.001',manual_quantity:'3.002',written_off_quantity:'5.003',movements_count:4 }
describe('product flow report validation',() => {
  it('keeps receipts, order consumption and losses separate, with exact large sums',() => {
    expect(flowRowSchema.safeParse(row).success).toBe(true)
    expect(flowRowSchema.safeParse({...row,received_quantity:'9999999999999999999999999.123'}).success).toBe(true)
    expect(flowRowSchema.safeParse({...row,written_off_quantity:'5.004'}).success).toBe(false)
    expect(flowRowSchema.safeParse({...row,ordered_quantity:'-20'}).success).toBe(false)
    expect(flowRowSchema.safeParse({...row,unit:'pcs'}).success).toBe(false)
  })
})
