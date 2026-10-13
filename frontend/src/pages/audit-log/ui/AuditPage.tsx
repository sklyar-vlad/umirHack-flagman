import { useState } from 'react'
import { useSelector } from 'react-redux'
import { useSearchParams } from 'react-router'
import { auditActions, auditEntities, auditObjectLabel, auditSearchError, auditSearchParams, changeAuditSearch, defaultAuditSearch, readAuditSearch, useAuditLogQuery, type AuditSearch } from '@/entities/audit-log'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, DataTable, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { AuditFilters } from './AuditFilters'
import { AuditDetails } from './AuditDetails'
import styles from './AuditPage.module.css'

export function AuditPage() {
  const [params, setParams] = useSearchParams(), values = readAuditSearch(params), filterError = auditSearchError(values)
  const { settings, user } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useAuditLogQuery(values, { skip: !!filterError || user?.role !== 'admin', refetchOnMountOrArgChange: true })
  const data = query.currentData, [selected, setSelected] = useState<string | null>(null), [resetEpoch, setResetEpoch] = useState(0)
  const entry = data?.items.find((item) => item.id === selected)
  const change = (patch: Partial<AuditSearch>) => { setSelected(null); setParams(auditSearchParams(changeAuditSearch(values, patch))) }
  const reset = () => { setSelected(null); setResetEpoch((value) => value + 1); setParams(new URLSearchParams()) }
  const filtered = Object.keys(defaultAuditSearch).some((key) => !['page', 'page_size', 'sort'].includes(key) && values[key as keyof AuditSearch] !== defaultAuditSearch[key as keyof AuditSearch])
  const empty = data?.total ? <EmptyState title="На этой странице нет записей" description="Вернитесь к первой странице." /> : filtered ? <NoResults query={values.search || 'Выбранные фильтры'} onReset={reset} /> : <EmptyState title="Журнал пока пуст" description="Здесь появятся действия пользователей и системы." />
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Журнал действий</h1><p className="text-muted">Администрирование ресторана</p></div><Button variant="secondary" disabled={query.isFetching || !!filterError} onClick={() => { void query.refetch() }}>Обновить журнал</Button></header>
    <AuditFilters key={JSON.stringify([params.toString(), resetEpoch])} values={values} timezone={timezone} onChange={change} onReset={reset} />
    {filterError ? <ErrorState title="Некорректные фильтры" description={filterError} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка журнала" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить журнал" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status" className="text-caption text-muted">Найдено записей: {data.total}. Страница {data.page}.</p>
        <DataTable rows={data.items} rowKey={(item) => item.id} caption="События журнала действий" emptyState={empty} loading={query.isFetching} columns={[
          { key: 'time', header: 'Время', render: (item) => <time dateTime={item.created_at}>{formatRestaurantDateTime(item.created_at, timezone)}</time> },
          { key: 'actor', header: 'Пользователь', render: (item) => <Button variant="ghost" onClick={() => change({ actor_id: item.actor?.id ?? 'system' })} aria-label={`Действия автора: ${item.actor?.name ?? 'Система'}`}>{item.actor?.name ?? 'Система'}</Button> },
          { key: 'action', header: 'Действие', render: (item) => auditActions[item.action] },
          { key: 'object', header: 'Объект', render: (item) => <div className={styles.object}><strong>{auditEntities[item.entity_type]}</strong><span>{auditObjectLabel(item)}</span><Button variant="ghost" onClick={() => setSelected(item.id)} aria-label={`Изменения записи ${item.id}`}>Посмотреть изменения</Button></div> },
        ]} />
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
        {entry && <AuditDetails entry={entry} onClose={() => setSelected(null)} />}
      </>}
    </ApiAccessBoundary>}
    <p className="text-caption text-muted">Записи журнала доступны только для просмотра.</p>
  </section>
}
