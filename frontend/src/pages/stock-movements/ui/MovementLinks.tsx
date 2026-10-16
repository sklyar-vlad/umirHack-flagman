import { Link } from 'react-router'
import { useLazyMovementSupplyQuery, type StockMovement } from '@/entities/stock-movement'
import { ApiAccessBoundary } from '@/features/access'
import { Button } from '@/shared/ui'

export function MovementLinks({ movement, orders }: { movement: StockMovement; orders: boolean }) {
  const [read, query] = useLazyMovementSupplyQuery()
  return <div className="ui-actions">
    <Link to="/inventory">На склад</Link>
    {orders && movement.order_id && <Link to={`/orders/${movement.order_id}`}>К заказу</Link>}
    <ApiAccessBoundary error={query.error}>
      {query.currentData ? <><Link to={`/supplies/${query.currentData.supply.id}`}>Поставка №{query.currentData.supply.number}</Link><Link to={`/inventory?${new URLSearchParams({ product_id: movement.product_id, supply_id: query.currentData.supply.id })}`}>Партии на складе</Link></> : <Button variant="ghost" loading={query.isFetching} onClick={() => { void read({ product_id: movement.product_id, batch_id: movement.batch_id }, false) }}>Найти поставку</Button>}
      {query.error && <p role="alert">Не удалось найти поставку партии. Исторические значения сохранены; повторите поиск или откройте склад.</p>}
      {query.isSuccess && query.currentData === null && <p>Партия недоступна. Историческая запись остаётся в журнале.</p>}
    </ApiAccessBoundary>
  </div>
}
