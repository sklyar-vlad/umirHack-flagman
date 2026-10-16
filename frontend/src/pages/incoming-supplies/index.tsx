import { useState } from 'react'
import { useSelector } from 'react-redux'
import { Link } from 'react-router'
import { selectSession } from '@/entities/session'
import { useIncomingSuppliesQuery, useAcceptIncomingMutation, type IncomingSupply, type IncomingSearch } from '@/entities/integration'
import { ApiAccessBoundary } from '@/features/access'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { formatQuantity, parseQuantity, quantityFromInput, quantityToInput, serializeQuantity } from '@/shared/lib/quantity'
import { Badge, Button, Dialog, EmptyState, ErrorState, FormField, Input, Pagination, Select, Skeleton } from '@/shared/ui'
import styles from './Incoming.module.css'
const labels = { pending: 'Ожидает приёмки', partial: 'Принята частично', received: 'Принята полностью' }
function Reception({ doc, onClose, onAccepted }: { doc: IncomingSupply; onClose: () => void; onAccepted: () => void }) {
  const [quantities,setQuantities] = useState<Record<string,string>>(() => Object.fromEntries(doc.items.map((line) => [line.id, quantityToInput(serializeQuantity(parseQuantity(line.quantity)!-parseQuantity(line.accepted_quantity)!),line.unit) ?? '0'])))
  const [accept, mutation] = useAcceptIncomingMutation(), [error,setError] = useState('')
  return <Dialog open title={`Принять поставку №${doc.number}`} onClose={() => { if (!mutation.isLoading) onClose() }}>
    <form className={styles.form} onSubmit={(event) => {
      event.preventDefault(); setError('')
      try {
        const items = doc.items.flatMap((line) => {
          const raw = quantities[line.id]?.trim() || '0', quantity = quantityFromInput(raw,line.unit,line.unit,{ allowZero:true }), amount = parseQuantity(quantity)!
          if (amount === 0n) return []
          if (amount > parseQuantity(line.quantity)!-parseQuantity(line.accepted_quantity)!) throw new Error(`${line.product_name}: количество больше непринятого остатка`)
          return [{ id: line.id, quantity }]
        })
        if (!items.length) throw new Error('Укажите количество хотя бы по одной строке. Ноль оставляет строку на ожидании.')
        void accept({ id: doc.id, expected_revision: doc.revision, items }).unwrap().then(onAccepted).catch((error: { status?: number }) => setError(error.status === 409 ? 'Поставка уже изменена. Закройте окно и проверьте актуальные количества.' : 'Приёмка не подтверждена. Закройте окно и обновите список, чтобы проверить результат перед повтором. Проверьте также срок годности и активность продуктов.'))
      } catch (error) { setError(error instanceof Error ? error.message : 'Проверьте количества') }
    }}>
      <p>Укажите, сколько фактически принимаете сейчас. Непринятый остаток останется в ожидании.</p>
      {doc.items.map((line) => {
        const remaining = parseQuantity(line.quantity)!-parseQuantity(line.accepted_quantity)!
        return <FormField key={line.id} label={`${line.line_number}. ${line.product_name} (${({g:'г',ml:'мл',pcs:'шт.'})[line.unit]})`} hint={`Осталось принять: ${formatQuantity(serializeQuantity(remaining),line.unit)}`}>
          {(props) => <Input {...props} inputMode={line.unit === 'pcs' ? 'numeric' : 'decimal'} value={quantities[line.id]} disabled={remaining === 0n || mutation.isLoading} onChange={(event) => setQuantities((current) => ({ ...current,[line.id]:event.target.value }))} />}
        </FormField>
      })}
      {error && <p role="alert">{error}</p>}
      <div className="ui-actions"><Button type="submit" loading={mutation.isLoading}>Подтвердить приёмку</Button><Button variant="ghost" disabled={mutation.isLoading} onClick={onClose}>Оставить на ожидании</Button></div>
    </form>
  </Dialog>
}
export function IncomingSuppliesPage() {
  const [values,setValues] = useState<IncomingSearch>({ status:'all',page:1,page_size:20 }), [selected,setSelected] = useState<IncomingSupply>(), [notice,setNotice] = useState('')
  const { settings } = useSelector(selectSession), timezone = settings?.timezone ?? 'Europe/Moscow'
  const query = useIncomingSuppliesQuery(values,{ refetchOnMountOrArgChange:true })
  return <section className={styles.page}>
    <nav className="ui-actions" aria-label="Поставки"><Link className="ui-button ui-button--secondary" to="/supplies">История приёмки</Link><span className="ui-button ui-button--primary" aria-current="page">Входящие поставки</span></nav>
    <header className={styles.header}><div><h1>Входящие поставки</h1><p className="text-muted">Заявки от поставщиков. На склад попадает только фактически принятое количество.</p></div><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить поставки</Button></header>
    <FormField label="Статус поставки">{(props) => <Select {...props} value={values.status} onChange={(event) => setValues({ ...values,status:event.target.value as IncomingSearch['status'],page:1 })}><option value="all">Все статусы</option><option value="pending">Ожидают приёмки</option><option value="partial">Приняты частично</option><option value="received">Приняты полностью</option></Select>}</FormField>
    {notice && <p role="status">{notice}</p>}
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton rows={4} label="Загрузка входящих поставок" />}
      {query.error && <ErrorState title="Не удалось загрузить поставки" onRetry={() => { void query.refetch() }} />}
      {query.currentData && <><p className="text-muted">Поставок: {query.currentData.total}</p>{query.currentData.items.map((doc) => <article className={styles.card} key={doc.id}>
        <header className={styles.header}><div><h2>Поставка №{doc.number}</h2><p className="text-muted">{doc.source} · {formatRestaurantDateTime(doc.created_at,timezone)}</p></div><Badge tone={doc.status === 'received' ? 'success' : 'warning'}>{labels[doc.status]}</Badge></header>
        <div className={styles.lines}>{doc.items.map((line) => <div className={styles.line} key={line.id}><div><strong>{line.product_name}</strong><p className="text-muted">Годен до {formatRestaurantDateTime(line.expires_at,timezone)}</p></div><p>Ожидалось: {formatQuantity(line.quantity,line.unit)}</p><p>Принято: <strong>{formatQuantity(line.accepted_quantity,line.unit)}</strong></p></div>)}</div>
        <div className="ui-actions">{doc.status !== 'received' && <Button disabled={!!query.error || query.isFetching} onClick={() => { setNotice(''); setSelected(doc) }}>Принять полностью или частично</Button>}{doc.receipt_ids.map((id,index) => <Link key={id} to={`/supplies/${id}`}>Приёмка {index+1}</Link>)}</div>
      </article>)}{!query.currentData.items.length && <EmptyState title="Входящих поставок нет" description="Здесь появятся заявки, отправленные внешним сервисом через API." />}
      {query.currentData.total > values.page_size && <Pagination page={values.page} pageCount={Math.max(values.page,Math.ceil(query.currentData.total/values.page_size))} onPageChange={(page) => setValues({ ...values,page })} />}</>}
    </ApiAccessBoundary>
    {selected && <Reception doc={selected} onClose={() => setSelected(undefined)} onAccepted={() => { setSelected(undefined); setNotice('Приёмка подтверждена. Принятые продукты добавлены на склад, остаток заявки сохранён.') }} />}
  </section>
}
