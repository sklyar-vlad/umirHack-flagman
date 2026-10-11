import type { OrderDetail } from '@/entities/order'

/** Published OpenAPI OrderDetail example; synthetic data, never a real server result. */
export const orderDetailFixture: OrderDetail = {
  id: '77777777-7777-4777-8777-777777777777', number: '247', status: 'completed', created_at: '2026-10-09T09:00:00Z',
  created_by: { id: '88888888-8888-4888-8888-888888888888', name: 'Оператор' },
  items: [{
    id: '78787878-7878-4787-8787-787878787878', dish_id: '66666666-6666-4666-8666-666666666666', dish_name: 'Пицца «Маргарита»', portions: 2, recipe_version: 1,
    allocations: [
      { movement_id: '79797979-7979-4797-8797-797979797979', product_id: '11111111-1111-4111-8111-111111111111', product_name: 'Моцарелла', unit: 'g', batch_id: '33333333-3333-4333-8333-333333333333', supply_id: '44444444-4444-4444-8444-444444444444', supply_number: '104', line_number: 1, quantity: '240.000', batch_remaining_quantity_after: '960.000' },
      { movement_id: '80808080-8080-4808-8808-808080808080', product_id: '10101010-1010-4010-8010-101010101010', product_name: 'Томаты', unit: 'g', batch_id: '12121212-1212-4212-8212-121212121212', supply_id: '44444444-4444-4444-8444-444444444444', supply_number: '104', line_number: 2, quantity: '200.000', batch_remaining_quantity_after: '800.000' },
      { movement_id: '81818181-8181-4818-8818-818181818181', product_id: '20202020-2020-4020-8020-202020202020', product_name: 'Тесто для пиццы', unit: 'pcs', batch_id: '13131313-1313-4313-8313-131313131313', supply_id: '44444444-4444-4444-8444-444444444444', supply_number: '104', line_number: 3, quantity: '2.000', batch_remaining_quantity_after: '8.000' },
    ],
  }],
}

export const splitOrderDetailFixture: OrderDetail = {
  ...orderDetailFixture,
  items: [{ ...orderDetailFixture.items[0], allocations: [
    { ...orderDetailFixture.items[0].allocations[0], quantity: '120.001', batch_remaining_quantity_after: '0.000' },
    { ...orderDetailFixture.items[0].allocations[0], movement_id: '91919191-9191-4919-8919-919191919191', batch_id: '92929292-9292-4929-8929-929292929292', supply_id: '93939393-9393-4939-8939-939393939393', supply_number: '105', line_number: 4, quantity: '119.999', batch_remaining_quantity_after: '840.001' },
    ...orderDetailFixture.items[0].allocations.slice(1),
  ] }],
}
