import { useSelector } from 'react-redux'
import { Link, useSearchParams } from 'react-router'
import { changeOrderSearch, defaultOrderSearch, orderSearchError, orderSearchParams, readOrderSearch, useOrderAttemptsQuery, useOrdersQuery, type AttemptSearch, type OrderTab } from '@/entities/order'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { DEFAULT_RESTAURANT_TIMEZONE } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { OrderFilters } from './OrderFilters'
import { AttemptRecords, OrderRecords } from './OrderRecords'
import styles from './OrdersPage.module.css'

export function OrdersPage() {
  const [params, setParams] = useSearchParams()
  const values = readOrderSearch(params), { tab } = values, current = values[tab], filterError = orderSearchError(current)
  const { settings, user } = useSelector(selectSession)
  const timezone = settings?.timezone ?? DEFAULT_RESTAURANT_TIMEZONE
  const orders = useOrdersQuery(values.orders, { skip: tab !== 'orders' || !!filterError, refetchOnMountOrArgChange: true })
  const attempts = useOrderAttemptsQuery(values.attempts, { skip: tab !== 'attempts' || !!filterError, refetchOnMountOrArgChange: true })
  const query = tab === 'orders' ? orders : attempts, data = query.currentData
  const change = (patch: Partial<AttemptSearch>) => setParams(orderSearchParams(changeOrderSearch(values, patch)))
  const reset = () => setParams(orderSearchParams({ ...values, [tab]: { ...defaultOrderSearch[tab] } }))
  const selectTab = (next: OrderTab) => setParams(orderSearchParams({ ...values, tab: next }))
  const returnTo = `/orders${orderSearchParams(values).size ? `?${orderSearchParams(values)}` : ''}`
  const canCreate = !!user && canAccess(user.role, sectionForPath('/orders')!)
  const noun = tab === 'orders' ? 'заказов' : 'отказов'
  const emptyState = data && data.total > 0 ? <EmptyState title="На этой странице нет записей" description="Вернитесь к первой странице списка." />
    : current.search || current.from || current.to || tab === 'attempts' && values.attempts.reason_code ? <NoResults query={current.search} onReset={reset} />
      : <EmptyState title={tab === 'orders' ? 'Заказов пока нет' : 'Отклонённых попыток пока нет'} description={tab === 'orders' ? 'История появится после успешного создания заказа.' : 'Здесь появятся только сохранённые предметные отказы при создании заказа.'} />
  return <div className={styles.page}>
    <header className={styles.header}><div><h1>Заказы</h1><p className="text-muted">Успешные заказы и сохранённые отклонённые попытки.</p></div><div className={styles.actions}>
      {canCreate && <Link className="ui-button ui-button--primary" to={`/orders/new?returnTo=${encodeURIComponent(returnTo)}`}>Новый заказ</Link>}
      <Button variant="secondary" disabled={query.isFetching || !!filterError} onClick={() => { void query.refetch() }}>Обновить историю</Button>
    </div></header>
    <div className={styles.tabs} role="tablist" aria-label="История заказов">{(['orders', 'attempts'] as const).map((key, index) => <button type="button" role="tab" id={`history-tab-${key}`} aria-controls={`history-panel-${key}`} aria-selected={tab === key} tabIndex={tab === key ? 0 : -1} key={key}
      onClick={() => selectTab(key)} onKeyDown={(event) => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
        event.preventDefault()
        const next = event.key === 'Home' ? 'orders' : event.key === 'End' ? 'attempts' : index === 0 ? 'attempts' : 'orders'
        selectTab(next); document.getElementById(`history-tab-${next}`)?.focus()
      }}>{key === 'orders' ? 'Успешные заказы' : 'Отклонённые попытки'}</button>)}</div>
    <section role="tabpanel" id={`history-panel-${tab}`} aria-labelledby={`history-tab-${tab}`} className={styles.panel}>
      <OrderFilters key={JSON.stringify([tab, current.search, current.from, current.to, values.attempts.reason_code])} tab={tab} values={{ ...current, reason_code: values.attempts.reason_code }} timezone={timezone} onChange={change} onReset={reset} />
      {tab === 'attempts' && <p className="text-caption text-muted">У попытки есть собственный ID и причина, без номера успешного заказа. Предварительная проверка и технические ошибки не создают запись в этой истории.</p>}
      {filterError ? <ErrorState title="Некорректные фильтры" description={filterError} /> : <ApiAccessBoundary error={query.error}>
        {query.isFetching && !query.currentData && <Skeleton label={`Загрузка ${noun}`} rows={4} />}
        {query.error && <ErrorState title={`Не удалось загрузить историю ${noun}`} description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз. Ошибка загрузки не является сохранённым отказом заказа.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
        {data && <>
          <p role="status" className="text-caption text-muted">Найдено {noun}: {data.total}. Страница {data.page}.</p>
          {tab === 'orders' && orders.currentData && <OrderRecords items={orders.currentData.items} timezone={timezone} returnTo={returnTo} loading={query.isFetching} emptyState={emptyState} />}
          {tab === 'attempts' && attempts.currentData && <AttemptRecords items={attempts.currentData.items} timezone={timezone} returnTo={returnTo} loading={query.isFetching} emptyState={emptyState} />}
          {data.total > data.page_size && <Pagination page={current.page} pageCount={Math.max(current.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
          {data.items.length === 0 && current.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
        </>}
      </ApiAccessBoundary>}
    </section>
  </div>
}
