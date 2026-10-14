import { useSelector } from 'react-redux'
import { useSearchParams } from 'react-router'
import { changeStopListSearch, defaultStopListSearch, readStopListSearch, stopListSearchParams, useStopListQuery, type StopListSearch } from '@/entities/dish'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { StopListFilters } from './StopListFilters'
import { StopListCard } from './StopListCard'
import styles from './StopListPage.module.css'

export function StopListPage() {
  const [params, setParams] = useSearchParams(), values = readStopListSearch(params)
  const timezone = useSelector(selectSession).settings?.timezone ?? 'Europe/Moscow'
  const query = useStopListQuery(values, { refetchOnMountOrArgChange: true })
  const data = query.currentData
  const change = (patch: Partial<StopListSearch>) => setParams(stopListSearchParams(changeStopListSearch(values, patch)))
  const reset = () => setParams(stopListSearchParams(defaultStopListSearch))
  return <div className={styles.page}>
    <header className={styles.row}><div><h1>Стоп-лист</h1><p className="text-muted">Активные блюда, для которых сейчас нельзя приготовить одну порцию.</p></div><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить стоп-лист</Button></header>
    <p className="text-caption text-muted">Отключённые блюда не входят в автоматический стоп-лист. Данные обновляются при серверных изменениях и восстановлении связи.</p>
    <StopListFilters key={values.search} values={values} onChange={change} onReset={reset} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка стоп-листа" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить стоп-лист" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status" className="text-caption text-muted">Недоступных блюд: {data.total}. Страница {data.page}. Снимок: <time dateTime={data.meta.server_time}>{formatRestaurantDateTime(data.meta.server_time, timezone)}</time>.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((dish) => <StopListCard key={dish.id} dish={dish} />)}</div> : data.total > 0 || values.page > 1 ? <EmptyState title="На этой странице нет блюд" description="Вернитесь к первой странице списка." /> : values.search ? <NoResults query={values.search} onReset={reset} /> : <EmptyState title="Все блюда доступны" description="Среди активных блюд нет недоступных. Отключённые блюда не участвуют в этом списке." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {data.items.length === 0 && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
