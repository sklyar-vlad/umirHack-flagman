import { Link } from 'react-router'
import type { OrderDetail } from '@/entities/order'
import { Button } from '@/shared/ui'
import { OrderSnapshot } from './OrderSnapshot'

export function CreatedOrder({ order, onNew }: { order: OrderDetail; onNew: () => void }) {
  return <div><p>Заказ подтверждён сервером. Корзина очищена.</p>
    <OrderSnapshot order={order} />
    <Link className="ui-button ui-button--secondary" to={`/orders/${order.id}`}>К заказу №{order.number}</Link> <Button onClick={onNew}>Создать ещё заказ</Button>
  </div>
}
