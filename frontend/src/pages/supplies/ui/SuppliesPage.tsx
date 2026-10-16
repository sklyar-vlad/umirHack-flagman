import { useSelector } from 'react-redux'
import { Link, useSearchParams } from 'react-router'
import { changeSupplySearch, defaultSupplySearch, readSupplySearch, supplyPeriodError, supplySearchParams, useSuppliesQuery, type SupplySearch, type SupplySummary } from '@/entities/supply'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { DEFAULT_RESTAURANT_TIMEZONE, formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, DataTable, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { SupplyFilters } from './SupplyFilters'
import styles from './SuppliesPage.module.css'

export function SuppliesPage() {
  const [params, setParams] = useSearchParams()
  const values = readSupplySearch(params), periodError = supplyPeriodError(values)
  const { settings, user } = useSelector(selectSession)
  const timezone = settings?.timezone ?? DEFAULT_RESTAURANT_TIMEZONE
  const query = useSuppliesQuery(values, { skip: !!periodError, refetchOnMountOrArgChange: true })
  // currentData belongs only to these filters. It survives a failed refresh but never leaks another list.
  const data = query.currentData
  const change = (patch: Partial<SupplySearch>) => setParams(supplySearchParams(changeSupplySearch(values, patch)))
  const reset = () => setParams(supplySearchParams(defaultSupplySearch))
  const returnTo = `/supplies${supplySearchParams(values).size ? `?${supplySearchParams(values)}` : ''}`
  const canReceive = !!user && canAccess(user.role, sectionForPath('/supplies')!)
  const columns = [
    { key: 'number', header: 'Номер', render: (item: SupplySummary) => <Link to={`/supplies/${item.id}?returnTo=${encodeURIComponent(returnTo)}`} aria-label={`Поставка ${item.number}`}>{item.number}</Link> },
    { key: 'created', header: 'Создана', render: (item: SupplySummary) => <time dateTime={item.created_at}>{formatRestaurantDateTime(item.created_at, timezone)}</time> },
    { key: 'received', header: 'Принята', render: (item: SupplySummary) => <time dateTime={item.received_at}>{formatRestaurantDateTime(item.received_at, timezone)}</time> },
    { key: 'items', header: 'Позиций', render: (item: SupplySummary) => item.items_count },
    { key: 'author', header: 'Сотрудник', render: (item: SupplySummary) => item.created_by.name },
    { key: 'status', header: 'Статус', render: () => <Badge tone="success">Принята</Badge> },
  ]
  return <div className={styles.page}>
    <header className={styles.header}><div><h1>Поставки</h1><p className="text-muted">История приёмки продуктов. Даты показаны в часовом поясе ресторана.</p></div><div className={styles.actions}>
      {canReceive && <Link className="ui-button ui-button--primary" to={`/supplies/new?returnTo=${encodeURIComponent(returnTo)}`}>Новая поставка</Link>}
      <Button variant="secondary" disabled={query.isFetching || !!periodError} onClick={() => { void query.refetch() }}>Обновить поставки</Button>
    </div></header>
    <nav className="ui-actions" aria-label="Поставки"><span className="ui-button ui-button--primary" aria-current="page">История приёмки</span><Link className="ui-button ui-button--secondary" to="/supplies/incoming">Входящие поставки</Link></nav>
    <SupplyFilters key={JSON.stringify([values.search, values.from, values.to])} values={values} timezone={timezone} onChange={change} onReset={reset} />
    {periodError ? <ErrorState title="Некорректный период" description={periodError} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка поставок" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить поставки" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status" className="text-caption text-muted">Найдено поставок: {data.total}. Страница {data.page}.</p>
        <DataTable rows={data.items} columns={columns} rowKey={(item) => item.id} caption="История поставок" loading={query.isFetching} emptyState={data.total > 0
          ? <EmptyState title="На этой странице нет поставок" description="Вернитесь к первой странице списка." />
          : values.search || values.from || values.to ? <NoResults query={values.search} onReset={reset} />
            : <EmptyState title="Поставок пока нет" description="История появится после первой приёмки продуктов." />} />
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {data.items.length === 0 && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>}
  </div>
}
