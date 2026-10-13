import { useState } from 'react'
import { useSelector } from 'react-redux'
import { useSearchParams } from 'react-router'
import { movementNames, useStockMovementsQuery, movementSearchError, readMovementSearch, movementSearchParams, defaultMovementSearch, changeMovementSearch, type MovementSearch } from '@/entities/stock-movement'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatMovementQuantity, formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, DataTable, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { MovementFilters } from './MovementFilters'
import { MovementLinks } from './MovementLinks'
import styles from './MovementsPage.module.css'

export function MovementsPage() {
  const [params, setParams] = useSearchParams(), values = readMovementSearch(params), error = movementSearchError(values)
  const { user, settings } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useStockMovementsQuery(values, { skip: !!error || !user || user.role === 'order_operator', refetchOnMountOrArgChange: true })
  const data = query.currentData, [resetEpoch, setResetEpoch] = useState(0)
  const change = (patch: Partial<MovementSearch>) => setParams(movementSearchParams(changeMovementSearch(values, patch)))
  const reset = () => { setResetEpoch((value) => value + 1); setParams(new URLSearchParams()) }
  const filtered = Object.keys(defaultMovementSearch).some((key) => !['page', 'page_size', 'sort'].includes(key) && values[key as keyof MovementSearch] !== defaultMovementSearch[key as keyof MovementSearch])
  const empty = data?.total ? <EmptyState title="На этой странице нет движений" description="Вернитесь к первой странице." /> : filtered ? <NoResults query="Выбранные фильтры" onReset={reset} /> : <EmptyState title="Движений пока нет" description="История появится после поставки, заказа или списания." />
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>История складских движений</h1><p className="text-muted">Приход, расход заказа и списания по партиям.</p></div><Button variant="secondary" disabled={query.isFetching || !!error} onClick={() => { void query.refetch() }}>Обновить историю</Button></header>
    <MovementFilters key={JSON.stringify([params.toString(), resetEpoch])} values={values} timezone={timezone} onChange={change} onReset={reset} />
    {error ? <ErrorState title="Некорректные фильтры" description={error} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка движений" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить движения" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status" className="text-caption text-muted">Найдено движений: {data.total}. Страница {data.page}.</p><DataTable rows={data.items} rowKey={(item) => item.id} caption="Записи складского учёта" emptyState={empty} loading={query.isFetching} columns={[
        { key: 'time', header: 'Дата', render: (item) => <time dateTime={item.occurred_at}>{formatRestaurantDateTime(item.occurred_at, timezone)}</time> },
        { key: 'product', header: 'Продукт / партия', render: (item) => <div className={styles.record}><strong>{item.product_name}</strong><span>ID партии: {item.batch_id}</span><Button variant="ghost" onClick={() => change({ product_id: item.product_id, batch_id: item.batch_id })}>Движения этой партии</Button></div> },
        { key: 'type', header: 'Тип', render: (item) => <Badge tone={item.type === 'receipt' ? 'success' : item.type === 'order_consumption' ? 'neutral' : 'warning'}>{movementNames[item.type]}</Badge> },
        { key: 'quantity', header: 'Изменение', render: (item) => formatMovementQuantity(item.delta_quantity, item.unit) },
        { key: 'remaining', header: 'Остаток после движения', render: (item) => formatQuantity(item.remaining_quantity_after, item.unit) },
        { key: 'actor', header: 'Автор / причина', render: (item) => <div className={styles.record}><span>{item.actor?.name ?? 'Система'}</span><span>{item.note ?? 'Причина не указана'}</span></div> },
        { key: 'links', header: 'Переходы', render: (item) => <MovementLinks movement={item} orders={user?.role === 'admin' || user?.role === 'manager'} /> },
      ]} />
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>}
    <p className="text-caption text-muted">Название, единица и остаток сохранены на момент операции. Расход заказа — использование ингредиентов, а не потеря.</p>
  </section>
}
