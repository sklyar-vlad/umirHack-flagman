import { expect,it } from 'vitest'
import { incomingSchema } from './index'
const line = {id:'00000000-0000-4000-8000-000000000001',product_id:'00000000-0000-4000-8000-000000000002',product_name:'Молоко',unit:'ml',quantity:'10.000',accepted_quantity:'4.000',expires_at:'2030-01-01T00:00:00Z',line_number:1}
const doc = {id:'00000000-0000-4000-8000-000000000003',external_id:'external-1',number:'П-1',source:'Поставщик',status:'partial',revision:2,created_at:'2026-10-15T00:00:00Z',items:[line],receipt_ids:['00000000-0000-4000-8000-000000000004']}
it('rejects over-receipt, inconsistent statuses, fractional pieces and repeated document lines',() => {
  expect(incomingSchema.safeParse(doc).success).toBe(true)
  expect(incomingSchema.safeParse({...doc,status:'received'}).success).toBe(false)
  expect(incomingSchema.safeParse({...doc,items:[{...line,accepted_quantity:'11'}]}).success).toBe(false)
  expect(incomingSchema.safeParse({...doc,items:[{...line,unit:'pcs',accepted_quantity:'1.5'}]}).success).toBe(false)
  expect(incomingSchema.safeParse({...doc,items:[line,line]}).success).toBe(false)
})
