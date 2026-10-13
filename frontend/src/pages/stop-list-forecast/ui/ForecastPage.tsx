import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { useSelector } from 'react-redux'
import { changeForecastSearch, readForecastSearch, forecastSearchError, forecastSearchParams, useStopListForecastQuery, type ForecastSearch } from '@/entities/stop-list-forecast'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, Pagination, Skeleton } from '@/shared/ui'
import { ForecastFilters } from './ForecastFilters'
import { ForecastCard } from './ForecastCard'
import styles from './ForecastPage.module.css'

export function ForecastPage() {
  const [params, setParams] = useSearchParams(), values = readForecastSearch(params), error = forecastSearchError(values)
  const { user, settings } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useStopListForecastQuery(values, { skip: !!error || !user || !['admin', 'manager'].includes(user.role), refetchOnMountOrArgChange: true, refetchOnFocus: true, refetchOnReconnect: true })
  const data = query.currentData, [resetEpoch, setResetEpoch] = useState(0)
  const change = (patch: Partial<ForecastSearch>) => setParams(forecastSearchParams(changeForecastSearch(values, patch)))
  const reset = () => { setResetEpoch((value) => value + 1); setParams(new URLSearchParams()) }
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Прогноз стоп-листа</h1><p className="text-muted">Оценка по текущим рецептам, остаткам, срокам партий и прошлому расходу. Это не гарантия доступности.</p></div><Button variant="secondary" disabled={query.isFetching || !!error} onClick={() => { void query.refetch() }}>Обновить прогноз</Button></header>
    <ForecastFilters key={JSON.stringify([params.toString(), resetEpoch])} values={values} onChange={change} onReset={reset} />
    <p className="text-caption text-muted">Будущие поставки не учитываются. Модель предполагает неизменную рецептуру и постоянный средний расход. Ближайшие оценки идут первыми; строки без даты — в конце, в порядке сервера.</p>
    {error ? <ErrorState title="Некорректные настройки" description={error} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка прогноза" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить прогноз" description={data ? 'Показан последний успешный прогноз для этих настроек. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status" className="text-caption text-muted">Активных блюд: {data.total}. Страница {data.page}. История: {data.meta.lookback_days} дн. Горизонт: {data.meta.horizon_hours} ч.</p>
        <p className="text-caption text-muted">Снимок: <time dateTime={data.meta.server_time}>{formatRestaurantDateTime(data.meta.server_time, timezone)}</time>. Часовой пояс: {timezone}.</p>
        {data.items.length && user ? <ul aria-label="Оценки остановки блюд" className={styles.cards}>{data.items.map((row) => <ForecastCard key={row.dish_id} row={row} timezone={timezone} role={user.role} />)}</ul> : <EmptyState title={data.total ? 'На этой странице нет блюд' : 'Активных блюд пока нет'} description={data.total ? 'Вернитесь к первой странице.' : 'Прогноз появится для активных блюд.'} />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>}
  </section>
}
