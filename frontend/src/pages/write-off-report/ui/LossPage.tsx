import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useSelector } from 'react-redux'
import { changeLossSearch, defaultLossSearch, lossMovementsUrl, lossSearchError, lossSearchParams, readLossSearch, useWriteOffReportQuery, type LossSearch } from '@/entities/write-off-report'
import { selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, DataTable, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { LossFilters } from './LossFilters'
import styles from './LossPage.module.css'

export function LossPage() {
  const [params, setParams] = useSearchParams(), values = readLossSearch(params), error = lossSearchError(values)
  const { user, settings } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useWriteOffReportQuery(values, { skip: !!error || !user || !['admin', 'manager'].includes(user.role), refetchOnMountOrArgChange: true })
  const data = query.currentData, [resetEpoch, setResetEpoch] = useState(0)
  const change = (patch: Partial<LossSearch>) => setParams(lossSearchParams(changeLossSearch(values, patch)))
  const reset = () => { setResetEpoch((value) => value + 1); setParams(new URLSearchParams()) }
  const filtered = values.product_id || values.from || values.to || values.reason !== defaultLossSearch.reason
  const empty = data?.total ? <EmptyState title="На этой странице нет потерь" description="Вернитесь к первой странице." /> : filtered ? <NoResults query="Выбранные фильтры" onReset={reset} /> : <EmptyState title="Потерь пока нет" description="Отчёт появится после ручного списания или списания просрочки. Расход заказа сюда не входит." />
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Потери от списаний</h1><p className="text-muted">Просрочка и ручные списания по продуктам и единицам.</p></div><Button variant="secondary" disabled={query.isFetching || !!error} onClick={() => { void query.refetch() }}>Обновить отчёт</Button></header>
    <LossFilters key={JSON.stringify([params.toString(), resetEpoch])} values={values} timezone={timezone} onChange={change} onReset={reset} />
    {error ? <ErrorState title="Некорректные фильтры" description={error} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка потерь" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить потери" description={data ? 'Показан последний успешный отчёт для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status" className="text-caption text-muted">Групп продуктов и единиц: {data.total}. Страница {data.page}.</p><DataTable rows={data.items} rowKey={(row) => `${row.product_id}:${row.unit}`} caption="Потери по продуктам" emptyState={empty} loading={query.isFetching} columns={[
        { key: 'product', header: 'Продукт', render: (row) => <div className={styles.record}><strong>{row.product_name}</strong><span>ID: {row.product_id}</span></div> },
        { key: 'unit', header: 'Единица', render: (row) => ({ g: 'г', ml: 'мл', pcs: 'шт.' })[row.unit] },
        { key: 'expired', header: 'Просрочка', render: (row) => formatQuantity(row.expired_written_off_quantity, row.unit) },
        { key: 'manual', header: 'Ручные списания', render: (row) => formatQuantity(row.manual_written_off_quantity, row.unit) },
        { key: 'total', header: 'Всего потерь', render: (row) => formatQuantity(row.total_written_off_quantity, row.unit) },
        { key: 'count', header: 'Движений', render: (row) => row.movements_count },
        { key: 'links', header: 'История', render: (row) => <div className={styles.record}>
          {values.reason !== 'manual_writeoff' && <Link to={lossMovementsUrl(row, values, 'expiration')}>Движения просрочки</Link>}
          {values.reason !== 'expiration' && <Link to={lossMovementsUrl(row, values, 'manual_writeoff')}>Ручные движения</Link>}
        </div> },
      ]} />
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>}
    <p className="text-caption text-muted">Названия сохранены в истории операций. Каждая строка — отдельный продукт и единица. Общего количества или денежной оценки нет. Расход ингредиентов на заказ не считается потерей.</p>
    <p className="text-caption text-muted">Ссылки сохраняют продукт, период и тип списания. История движений показывает все исторические единицы продукта: сверяйте единицу выбранной строки.</p>
  </section>
}
