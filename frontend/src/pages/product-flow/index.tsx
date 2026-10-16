import { useSelector } from 'react-redux'
import { Link, useSearchParams } from 'react-router'
import { useState } from 'react'
import { selectSession } from '@/entities/session'
import { readFlowSearch, flowError, useProductFlowQuery, type FlowSearch } from '@/entities/product-flow'
import { ApiAccessBoundary } from '@/features/access'
import { historyInstantToInput, historyFilterSchema } from '@/shared/lib/history'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, DataTable, EmptyState, ErrorState, FormField, Input, Pagination, Skeleton } from '@/shared/ui'
import styles from './ProductFlow.module.css'

function PeriodFilters({ values, timezone, change }: { values: FlowSearch; timezone: string; change: (patch: Partial<FlowSearch>) => void }) {
  const [from, setFrom] = useState(historyInstantToInput(values.from, timezone)), [to, setTo] = useState(historyInstantToInput(values.to, timezone)), [error, setError] = useState('')
  return <form className={styles.filters} onSubmit={(event) => {
    event.preventDefault()
    const parsed = historyFilterSchema(timezone, values).safeParse({ search: '', from, to })
    if (!parsed.success) { setError(parsed.error.issues[0]?.message ?? 'Проверьте период'); return }
    setError(''); change({ from: parsed.data.from, to: parsed.data.to })
  }}>
    <FormField label="Период с (включительно)">{(props) => <Input {...props} type="datetime-local" step="any" value={from} onChange={(event) => setFrom(event.target.value)} />}</FormField>
    <FormField label="Период до (не включая)">{(props) => <Input {...props} type="datetime-local" step="any" value={to} onChange={(event) => setTo(event.target.value)} />}</FormField>
    <div className="ui-actions"><Button type="submit">Применить</Button><Button variant="ghost" onClick={() => { setFrom(''); setTo(''); change({ from: '', to: '', product_id: '' }) }}>Сбросить</Button></div>
    {error && <p role="alert">{error}</p>}
  </form>
}
export function ProductFlowPage() {
  const [params, setParams] = useSearchParams(), values = readFlowSearch(params), error = flowError(values)
  const { user, settings } = useSelector(selectSession)
  const query = useProductFlowQuery(values, { skip: !!error || !user || !['admin', 'manager'].includes(user.role), refetchOnMountOrArgChange: true })
  const change = (patch: Partial<FlowSearch>) => { const next = { ...values, ...patch, page: patch.page ?? 1 }; setParams(Object.fromEntries(Object.entries(next).filter(([, value]) => value !== '').map(([key, value]) => [key, String(value)]))) }
  const data = query.currentData
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Движение продуктов</h1><p className="text-muted">Сколько поступило, использовано в заказах и списано за выбранный период.</p></div><Button variant="secondary" disabled={query.isFetching || !!error} onClick={() => { void query.refetch() }}>Обновить отчёт</Button></header>
    <PeriodFilters key={`${params}:${settings?.timezone}`} values={values} timezone={settings?.timezone ?? 'Europe/Moscow'} change={change} />
    <p className="text-caption text-muted">Часовой пояс: {settings?.timezone ?? 'Europe/Moscow'}. Учитываются принятые поставки и успешно оформленные заказы. Ожидающие поставки и отклонённые заказы не меняют показатели.</p>
    {error ? <ErrorState title="Некорректные фильтры" description={error} /> : <ApiAccessBoundary error={query.error}>
      {query.isFetching && !data && <Skeleton label="Загрузка движения продуктов" rows={5} />}
      {query.error && <ErrorState title="Не удалось загрузить отчёт" description={data ? 'Показан предыдущий отчёт. Данные могут быть устаревшими.' : 'Повторите запрос.'} onRetry={() => { void query.refetch() }} />}
      {data && <><p role="status">Продуктов и единиц: {data.total}</p><DataTable rows={data.items} rowKey={(row) => `${row.product_id}:${row.unit}`} caption="Приход и расход продуктов" loading={query.isFetching} emptyState={<EmptyState title="За этот период движений нет" description="Измените период или примите первую поставку." />} columns={[
        { key: 'product', header: 'Продукт', render: (row) => <strong>{row.product_name}</strong> },
        { key: 'received', header: 'Пришло в поставках', render: (row) => formatQuantity(row.received_quantity, row.unit) },
        { key: 'ordered', header: 'Ушло в заказы', render: (row) => formatQuantity(row.ordered_quantity, row.unit) },
        { key: 'expired', header: 'Просрочка', render: (row) => formatQuantity(row.expired_quantity, row.unit) },
        { key: 'manual', header: 'Ручное списание', render: (row) => formatQuantity(row.manual_quantity, row.unit) },
        { key: 'loss', header: 'Всего списано', render: (row) => formatQuantity(row.written_off_quantity, row.unit) },
        { key: 'history', header: 'Детали', render: (row) => <Link to={`/stock-movements?${new URLSearchParams({ product_id: row.product_id, ...(values.from ? { from: values.from } : {}), ...(values.to ? { to: values.to } : {}) })}`}>Движения</Link> },
      ]} />{data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} onPageChange={(page) => change({ page })} />}</>}
    </ApiAccessBoundary>}
    <p className="text-caption text-muted">Количество ингредиентов в заказах рассчитано по фактическому списанию партий. Разные продукты и единицы не суммируются в общий итог.</p>
  </section>
}
