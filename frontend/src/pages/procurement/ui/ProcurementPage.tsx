import { useSelector } from 'react-redux'
import { NavLink, useSearchParams } from 'react-router'
import { changeProcurementSearch, defaultProcurementSearch, readProcurementSearch, procurementSearchParams, useProcurementQuery, type ProcurementSearch } from '@/entities/procurement'
import { selectSession, visibleSections } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { ProcurementFilters } from './ProcurementFilters'
import { ProcurementCard } from './ProcurementCard'
import styles from './ProcurementPage.module.css'
export function ProcurementPage() {
  const [params, setParams] = useSearchParams(), values = readProcurementSearch(params)
  const session = useSelector(selectSession), timezone = session.settings?.timezone ?? 'Europe/Moscow'
  const query = useProcurementQuery(values, { refetchOnMountOrArgChange: true })
  const data = query.currentData
  const change = (patch: Partial<ProcurementSearch>) => setParams(procurementSearchParams(changeProcurementSearch(values, patch)))
  const reset = () => setParams(procurementSearchParams(defaultProcurementSearch))
  return <div className={styles.page}>
    <h1>Рекомендации</h1>
    <nav aria-label="Вкладки рекомендаций" className={styles.actions}>{visibleSections(session.user!.role, 'recommendations').map((section) => <NavLink key={section.id} to={section.path} className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`}>{section.title}</NavLink>)}</nav>
    <header className={styles.row}><div><h2>Закупить продукты</h2><p className="text-muted">Положительный дефицит относительно целевого запаса, без прогноза спроса.</p></div><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить закупки</Button></header>
    <p className="text-caption text-muted">Приоритет сервера: больше блокируемых блюд, затем отсутствие годного остатка, затем название. Просрочка не уменьшает закупку. Снимок обновляется при серверных изменениях и восстановлении связи.</p>
    <ProcurementFilters key={values.search} values={values} onChange={change} onReset={reset} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка закупок" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить закупки" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status" className="text-caption text-muted">Продуктов к закупке: {data.total}. Страница {data.page}. Снимок: <time dateTime={data.meta.server_time}>{formatRestaurantDateTime(data.meta.server_time, timezone)}</time>.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((item) => <ProcurementCard key={item.product_id} item={item} />)}</div> : data.total > 0 || values.page > 1 ? <EmptyState title="На этой странице нет закупок" description="Вернитесь к первой странице списка." /> : values.search || values.category_id ? <NoResults query={values.search || 'Выбранная категория'} onReset={reset} /> : <EmptyState title="Закупки не требуются" description="В списке только продукты с заданным положительным целевым запасом и дефицитом. Продукты без цели, с нулевой целью или достаточным годным остатком не дают рекомендации. Это не означает, что все блюда доступны." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {data.items.length === 0 && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
