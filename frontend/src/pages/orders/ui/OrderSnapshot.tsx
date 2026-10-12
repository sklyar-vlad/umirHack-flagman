import { Link } from 'react-router'
import { useSelector } from 'react-redux'
import type { OrderDetail } from '@/entities/order'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, DataTable } from '@/shared/ui'
import styles from './OrderDetailPage.module.css'

/** Historical server snapshot, shared by the creation result and detail read. */
export function OrderSnapshot({ order, loading = false }: { order: OrderDetail; loading?: boolean }) {
  const session = useSelector(selectSession)
  const allowed = (path: string) => !!session.user && canAccess(session.user.role, sectionForPath(path)!)
  const warehouse = allowed('/inventory'), supplies = allowed('/supplies')
  return <div className={styles.snapshot}>
    <div className={styles.summary}>
      <div className={styles.header}><h2>Заказ №{order.number}</h2><Badge tone="success">Выполнен</Badge></div>
      <dl className={styles.metrics}>
        <div><dt>Дата создания</dt><dd><time dateTime={order.created_at}>{formatRestaurantDateTime(order.created_at, session.settings?.timezone ?? 'Europe/Moscow')}</time></dd></div>
        <div><dt>Сотрудник</dt><dd>{order.created_by.name}</dd></div>
        <div><dt>Позиций</dt><dd>{order.items.length}</dd></div>
        <div><dt>Порций</dt><dd>{order.items.reduce((sum, item) => sum + item.portions, 0)}</dd></div>
      </dl>
      {allowed('/stock-movements') && <Link to={`/stock-movements?${new URLSearchParams({ order_id: order.id })}`}>История движений заказа</Link>}
      <p className="text-caption text-muted">Состав и списания сохранены на момент заказа. Остаток после движения не является текущим остатком на складе.</p>
    </div>
    {order.items.map((item) => <section className={styles.item} key={item.id} aria-label={`Позиция: ${item.dish_name}`}>
      <h3>{item.dish_name} — {item.portions} порц.</h3><p>Рецептура: {item.recipe_version}</p>
      <DataTable caption={`Фактические списания: ${item.dish_name}`} rows={item.allocations} rowKey={(row) => row.movement_id} loading={loading} emptyState={null} columns={[
        { key: 'product', header: 'Продукт', render: (row) => <strong>{row.product_name}</strong> },
        { key: 'batch', header: 'Партия', render: (row) => <span>{row.batch_id}</span> },
        { key: 'supply', header: 'Поставка / строка', render: (row) => <><span>Поставка {row.supply_number}, строка {row.line_number}</span><small>ID поставки: {row.supply_id}</small></> },
        { key: 'quantity', header: 'Списано', render: (row) => formatQuantity(row.quantity, row.unit) },
        { key: 'remaining', header: 'Остаток после движения', render: (row) => formatQuantity(row.batch_remaining_quantity_after, row.unit) },
        { key: 'warehouse', header: 'Склад', render: (row) => warehouse ? <Link to={`/inventory?${new URLSearchParams({ search: row.product_name, product_id: row.product_id, ...(supplies ? { supply_id: row.supply_id } : {}) })}`} aria-label={`Партия на складе: ${row.product_name}, поставка ${row.supply_number}, строка ${row.line_number}`}>Партия на складе</Link> : 'Нет доступа' },
      ]} />
    </section>)}
  </div>
}
