import { Link } from 'react-router'
import { useSelector } from 'react-redux'
import type { OrderDetail } from '@/entities/order'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, DataTable } from '@/shared/ui'

export function CreatedOrder({ order, onNew }: { order: OrderDetail; onNew: () => void }) {
  const session = useSelector(selectSession)
  const warehouse = !!session.user && canAccess(session.user.role, sectionForPath('/inventory')!)
  return <div><h2>Заказ №{order.number}</h2><p>{formatRestaurantDateTime(order.created_at, session.settings?.timezone ?? 'Europe/Moscow')} · {order.created_by.name}</p><p>Заказ подтверждён сервером. Корзина очищена.</p>
    {order.items.map((item) => <section key={item.id}><h3>{item.dish_name} — {item.portions} порц.</h3><p>Рецептура: {item.recipe_version}</p>
      <DataTable caption={`Фактические списания: ${item.dish_name}`} rows={item.allocations} rowKey={(row) => row.movement_id} emptyState={null} columns={[
        { key: 'product', header: 'Продукт', render: (row) => row.product_name },
        { key: 'batch', header: 'Партия', render: (row) => <><span>{row.batch_id}</span><p>Поставка {row.supply_number}, строка {row.line_number}</p></> },
        { key: 'quantity', header: 'Списано', render: (row) => formatQuantity(row.quantity, row.unit) },
        { key: 'remaining', header: 'Остаток после движения', render: (row) => formatQuantity(row.batch_remaining_quantity_after, row.unit) },
        { key: 'warehouse', header: 'Склад', render: (row) => warehouse ? <Link to={`/inventory?${new URLSearchParams({ search: row.product_name, product_id: row.product_id, supply_id: row.supply_id })}`}>Партия на складе</Link> : 'Нет доступа' },
      ]} />
    </section>)}
    <Link className="ui-button ui-button--secondary" to={`/orders/${order.id}`}>К заказу №{order.number}</Link> <Button onClick={onNew}>Создать ещё заказ</Button>
  </div>
}
