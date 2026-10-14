import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useSelector } from 'react-redux'
import { changeShortageSearch, readShortageSearch, shortageSearchError, shortageSearchParams, useIngredientShortagesQuery, type ShortageSearch } from '@/entities/ingredient-shortage'
import { defaultOrderSearch, orderSearchParams } from '@/entities/order'
import { selectSession, canAccess, sectionForPath } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, DataTable, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { ShortageFilters } from './ShortageFilters'
import styles from './IngredientShortagesPage.module.css'

export function IngredientShortagesPage() {
  const [params, setParams] = useSearchParams(), values = readShortageSearch(params), error = shortageSearchError(values)
  const { user, settings } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useIngredientShortagesQuery(values, { skip: !!error || !user || !['admin', 'manager'].includes(user.role), refetchOnMountOrArgChange: true, refetchOnFocus: false, refetchOnReconnect: false })
  const data = query.currentData, [resetEpoch, setResetEpoch] = useState(0)
  const change = (patch: Partial<ShortageSearch>) => setParams(shortageSearchParams(changeShortageSearch(values, patch)))
  const reset = () => { setResetEpoch((value) => value + 1); setParams(new URLSearchParams()) }
  const allowed = (path: string) => { const section = sectionForPath(path); return !!user && !!section && canAccess(user.role, section) }
  const attemptsUrl = `/orders?${orderSearchParams({ ...defaultOrderSearch, tab: 'attempts', attempts: { ...defaultOrderSearch.attempts, from: values.from, to: values.to, reason_code: 'INSUFFICIENT_STOCK' } })}`
  const empty = data?.total ? <EmptyState title="На этой странице нет недостачи" description="Вернитесь к первой странице." /> : values.from || values.to ? <NoResults query="Выбранный период" onReset={reset} /> : <EmptyState title="Отказов из-за недостачи пока нет" description="Отчёт появится после сохранённого отказа в оформлении заказа из-за нехватки ингредиентов." />
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Недостача ингредиентов</h1><p className="text-muted">Неудовлетворённый спрос из отклонённых заказов. Это не подтверждённые будущие заказы.</p></div><Button variant="secondary" disabled={query.isFetching || !!error} onClick={() => { void query.refetch() }}>Обновить отчёт</Button></header>
    <ShortageFilters key={JSON.stringify([params.toString(), resetEpoch])} values={values} timezone={timezone} onChange={change} onReset={reset} />
    {error ? <ErrorState title="Некорректные фильтры" description={error} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка недостачи" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить недостачу" description={data ? 'Показан последний успешный отчёт для этого периода. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status" className="text-caption text-muted">Групп продуктов и единиц: {data.total}. Страница {data.page}.</p><DataTable rows={data.items} rowKey={(row) => `${row.product_id}:${row.unit}`} caption="Недостача по продуктам" emptyState={empty} loading={query.isFetching} columns={[
        { key: 'product', header: 'Продукт', render: (row) => <div className={styles.record}><strong>{row.product_name}</strong><span>ID: {row.product_id}</span></div> },
        { key: 'unit', header: 'Единица', render: (row) => ({ g: 'г', ml: 'мл', pcs: 'шт.' })[row.unit] },
        { key: 'count', header: 'Отклонённых попыток', render: (row) => row.rejected_attempts_count },
        { key: 'missing', header: 'Суммарная недостача', render: (row) => formatQuantity(row.total_missing_quantity, row.unit) },
        { key: 'links', header: 'Подробнее', render: (row) => <div className={styles.record}>
          {allowed('/admin/products') && <Link to={`/admin/products/${row.product_id}/edit`}>Карточка продукта</Link>}
          {allowed('/inventory') && <Link to={`/inventory?${new URLSearchParams({ search: row.product_name, product_id: row.product_id })}`}>Продукт на складе</Link>}
          {allowed('/orders') && <Link to={attemptsUrl}>История отказов за период</Link>}
        </div> },
      ]} />
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>}
    <p className="text-caption text-muted">Каждая строка — продукт и историческая единица. Название сохранено на момент последнего отказа в периоде. Количества разных единиц не суммируются.</p>
    <p className="text-caption text-muted">Учитываются только сохранённые отказы из-за нехватки ингредиентов. Предварительные проверки, успешные заказы, другие причины отказа и повтор одного ключа не добавляют попытку.</p>
    <p className="text-caption text-muted">История отказов открывает все отказы из-за недостачи за выбранный период. Фильтра по продукту у истории нет. На складе используются текущие названия; исторический продукт может отсутствовать в остатках.</p>
  </section>
}
