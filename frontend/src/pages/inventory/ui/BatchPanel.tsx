import { useState } from 'react'
import { Link } from 'react-router'
import { useSelector } from 'react-redux'
import { useProductBatchesQuery, type BatchSearch, type InventoryItem } from '@/entities/inventory'
import { canAccess, sections, selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, EmptyState, ErrorState, FormField, Pagination, Select, Skeleton } from '@/shared/ui'
import { batchStatuses, remainingTime } from './inventory-format'
import styles from './InventoryPage.module.css'

export function BatchPanel({ item, supplyId = '' }: { item: { product: Pick<InventoryItem['product'], 'id' | 'name' | 'unit' | 'display_unit'> }; supplyId?: string }) {
  const [params, setParams] = useState<Omit<BatchSearch, 'productId'>>({ sort: 'expires_at', page: 1, page_size: 20, ...(supplyId && { status: 'all' }) })
  const query = useProductBatchesQuery({ productId: item.product.id, ...params })
  const session = useSelector(selectSession)
  const timezone = session.settings?.timezone
  const role = session.user?.role
  const permitted = (id: string) => !!role && canAccess(role, sections.find((section) => section.id === id)!)
  const data = query.currentData
  const unitMismatch = data?.items.some((batch) => batch.unit !== item.product.unit)
  return <section className={styles.batches} aria-label={`Партии: ${item.product.name}`} aria-busy={query.isFetching}>
    <h3>Партии продукта</h3>
    <div className={styles.batchControls}>
      <FormField label="Показать партии">{(props) => <Select {...props} value={params.status ?? ''} onChange={(event) => setParams({ ...params, status: event.target.value === '' ? undefined : event.target.value as BatchSearch['status'], page: 1 })}>
        <option value="">С остатком</option><option value="all">Вся история</option><option value="valid">Годные</option><option value="expiring">Скоро истекающие</option><option value="expired">Просроченные</option><option value="depleted">Израсходованные</option>
      </Select>}</FormField>
      <FormField label="Сортировка партий">{(props) => <Select {...props} value={params.sort} onChange={(event) => setParams({ ...params, sort: event.target.value === '-created_at' ? '-created_at' : 'expires_at', page: 1 })}>
        <option value="expires_at">FEFO: по сроку</option><option value="-created_at">Сначала новые записи</option>
      </Select>}</FormField>
      <Button variant="ghost" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить партии</Button>
    </div>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching ? <Skeleton label={`Загрузка партий: ${item.product.name}`} /> : query.error || unitMismatch ? <ErrorState title="Не удалось загрузить партии" description={unitMismatch ? 'Ответ сервера содержит несовместимые единицы.' : 'Список продуктов доступен. Повторите загрузку этой панели.'} onRetry={() => { void query.refetch() }} /> : data && <>
        <p className="text-caption text-muted">Найдено партий: {data.total}. Статусы на {formatRestaurantDateTime(data.meta.server_time, timezone)}.</p>
        {data.items.length === 0 ? <EmptyState title="Нет подходящих партий" description="Попробуйте другой фильтр истории. Израсходованные партии не входят в режим «С остатком»." /> : <ul className={styles.batchList}>{data.items.map((batch) => <li key={batch.id} className={styles.batch}>
          <div className={styles.row}>{supplyId === batch.supply.id && <Badge>Выбранная поставка</Badge>}<strong>{batch.supply.number} · строка {batch.line_number}</strong><Badge tone={batch.status === 'expired' ? 'danger' : batch.status === 'expiring' ? 'warning' : batch.status === 'valid' ? 'success' : 'neutral'}>{batchStatuses[batch.status]}</Badge></div>
          <dl className={styles.metrics}>
            <div><dt>Остаток партии</dt><dd>{formatQuantity(batch.remaining_quantity, batch.unit, item.product.display_unit)}</dd></div>
            <div><dt>Принято</dt><dd>{formatQuantity(batch.initial_quantity, batch.unit, item.product.display_unit)}</dd></div>
            <div><dt>Срок партии</dt><dd>{formatRestaurantDateTime(batch.expires_at, timezone)}<small>{remainingTime(batch.remaining_seconds, batch.status === 'expired')}</small></dd></div>
            <div><dt>Приёмка поставки</dt><dd>{formatRestaurantDateTime(batch.supply.received_at, timezone)}</dd></div>
          </dl>
          <p className="text-caption text-muted">ID партии: {batch.id}</p>
          <div className={styles.actions}>
            {permitted('movements') && <Link to={`/stock-movements?${new URLSearchParams({ product_id: batch.product_id, batch_id: batch.id })}`}>История партии · строка {batch.line_number}</Link>}
            {permitted('supplies') && <Link to={`/supplies/${batch.supply.id}`}>К поставке {batch.supply.number}, строка {batch.line_number}</Link>}
            {permitted('write-offs') && batch.status !== 'depleted' && <Link to={`/write-offs?${new URLSearchParams({ product_id: batch.product_id, batch_id: batch.id })}`}>Списать партию · строка {batch.line_number}</Link>}
          </div>
        </li>)}</ul>}
        {data.total > data.page_size && <Pagination page={params.page} pageCount={Math.max(params.page, Math.ceil(data.total / data.page_size))} onPageChange={(page) => setParams({ ...params, page })} />}
        {data.items.length === 0 && params.page > 1 && <Button variant="secondary" onClick={() => setParams({ ...params, page: 1 })}>К первой странице партий</Button>}
      </>}
    </ApiAccessBoundary>
  </section>
}
