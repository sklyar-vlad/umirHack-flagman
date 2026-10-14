import { isApiError } from '@/shared/api'
import { useSelector } from 'react-redux'
import { Link, useParams, useSearchParams } from 'react-router'
import { safeSupplyListUrl, useSupplyQuery, type SupplyLine } from '@/entities/supply'
import { supplyInventoryUrl } from '@/entities/inventory'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, DataTable, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import styles from './SupplyDetailPage.module.css'

const statuses = { valid: 'Годная', expiring: 'Скоро истекает', expired: 'Просрочена', depleted: 'Израсходована' }
export function SupplyDetailPage() {
  const { id = '' } = useParams()
  const [params] = useSearchParams()
  const timezone = useSelector(selectSession).settings?.timezone
  const query = useSupplyQuery(id, { refetchOnMountOrArgChange: true })
  const data = query.currentData
  const errorStatus = isApiError(query.error) ? query.error.status : undefined
  const unavailable = errorStatus === 404 || errorStatus === 422
  const date = (value: string) => <time dateTime={value}>{formatRestaurantDateTime(value, timezone)}</time>
  const columns = [
    { key: 'line', header: 'Строка', render: ({ batch }: SupplyLine) => batch.line_number },
    { key: 'product', header: 'Продукт', render: ({ product, batch }: SupplyLine) => <><strong>{product.name}</strong><small>ID партии: {batch.id}</small></> },
    { key: 'initial', header: 'Принято', render: ({ product, batch }: SupplyLine) => formatQuantity(batch.initial_quantity, batch.unit, product.display_unit) },
    { key: 'remaining', header: 'Текущий остаток', render: ({ product, batch }: SupplyLine) => formatQuantity(batch.remaining_quantity, batch.unit, product.display_unit) },
    { key: 'expiry', header: 'Годна до', render: ({ batch }: SupplyLine) => date(batch.expires_at) },
    { key: 'status', header: 'Статус партии', render: ({ batch }: SupplyLine) => <Badge tone={batch.status === 'expired' ? 'danger' : batch.status === 'expiring' ? 'warning' : batch.status === 'valid' ? 'success' : 'neutral'}>{statuses[batch.status]}</Badge> },
    { key: 'inventory', header: 'Склад', render: ({ product, batch }: SupplyLine) => <Link to={supplyInventoryUrl(product, batch.supply.id)} aria-label={`На склад: ${product.name}, строка ${batch.line_number}`}>На склад</Link> },
  ]
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Детали поставки</h1><p className="text-muted">Принятая поставка доступна только для просмотра.</p></div>
      <Link className="ui-button ui-button--secondary" to={safeSupplyListUrl(params.get('returnTo'))}>К истории поставок</Link>
    </header>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка поставки" rows={4} />}
      {unavailable ? <EmptyState title={errorStatus === 404 ? 'Поставка не найдена' : 'Некорректный адрес поставки'} description={errorStatus === 404 ? 'Эта поставка отсутствует. Вернитесь к истории поставок.' : 'Проверьте идентификатор в ссылке или выберите поставку из истории.'} /> : <>
        {query.error && <ErrorState title="Не удалось загрузить поставку" description={data ? 'Показан последний успешный снимок этой поставки. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
        {data && <>
          <div className={styles.summary}>
            <div className={styles.header}><h2>Поставка {data.number}</h2><Badge tone="success">Принята</Badge></div>
            <dl className={styles.metrics}>
              <div><dt>Фактическая приёмка</dt><dd>{date(data.received_at)}</dd></div>
              <div><dt>Запись создана</dt><dd>{date(data.created_at)}</dd></div>
              <div><dt>Сотрудник</dt><dd>{data.created_by.name}</dd></div>
              <div><dt>Исходных строк</dt><dd>{data.items_count}</dd></div>
            </dl>
            <Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить поставку</Button>
          </div>
          <p role="status" className="text-caption text-muted">Остатки и статусы на {formatRestaurantDateTime(data.meta.server_time, timezone)}. Израсходованные партии сохранены в истории. Количества показаны в единицах продукта.</p>
          <DataTable rows={data.items} columns={columns} rowKey={({ batch }) => batch.id} caption="Состав поставки" loading={query.isFetching} emptyState={<EmptyState title="Нет строк поставки" />} />
        </>}
      </>}
    </ApiAccessBoundary>
  </section>
}
