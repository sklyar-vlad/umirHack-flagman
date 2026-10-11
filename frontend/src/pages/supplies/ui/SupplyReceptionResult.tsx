import { Link } from 'react-router'
import { supplyInventoryUrl } from '@/entities/inventory'
import type { SupplyDetail } from '@/entities/supply'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, DataTable, EmptyState } from '@/shared/ui'
import styles from './SupplyDetailPage.module.css'

export function SupplyReceptionResult({ supply, timezone, returnTo }: { supply: SupplyDetail; timezone: string; returnTo: string }) {
  return <div className={styles.page}>
    <div role="status"><h2>Поставка принята</h2><p>Номер: {supply.number}. Принято строк: {supply.items_count}.</p><Badge tone="success">Принята</Badge></div>
    <p>Фактическая приёмка: {formatRestaurantDateTime(supply.received_at, timezone)}. Запись создана: {formatRestaurantDateTime(supply.created_at, timezone)}.</p>
    <Link className="ui-button ui-button--primary" to={`/supplies/${supply.id}?${new URLSearchParams({ returnTo })}`}>Детали принятой поставки</Link>
    <DataTable rows={supply.items} rowKey={({ batch }) => batch.id} caption="Принятые партии" emptyState={<EmptyState title="Нет принятых строк" />} columns={[
      { key: 'line', header: 'Строка', render: ({ batch }) => batch.line_number },
      { key: 'product', header: 'Продукт / партия', render: ({ product, batch }) => <><strong>{product.name}</strong><small>ID партии: {batch.id}</small></> },
      { key: 'quantity', header: 'Принято', render: ({ product, batch }) => formatQuantity(batch.initial_quantity, batch.unit, product.display_unit) },
      { key: 'expiry', header: 'Годна до', render: ({ batch }) => formatRestaurantDateTime(batch.expires_at, timezone) },
      { key: 'inventory', header: 'Склад', render: ({ product, batch }) => <Link to={supplyInventoryUrl(product, supply.id)} aria-label={`На склад: ${product.name}, строка ${batch.line_number}`}>На склад</Link> },
    ]} />
  </div>
}
