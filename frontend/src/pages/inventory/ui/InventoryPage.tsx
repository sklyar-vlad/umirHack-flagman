import { useSelector } from 'react-redux'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { useSearchParams } from 'react-router'
import { readInventorySelection, inventorySelectionParams, changeInventorySearch, defaultInventorySearch, inventoryFilters, inventorySearchParams, readInventorySearch, useInventoryQuery, type InventoryFilter, type InventorySearch } from '@/entities/inventory'
import { ApiAccessBoundary } from '@/features/access'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { InventorySelection } from './InventorySelection'
import { InventoryFilters } from './InventoryFilters'
import { InventoryCard } from './InventoryCard'
import styles from './InventoryPage.module.css'

const filterLabels: Record<InventoryFilter, string> = { all: 'Все', expiring: 'Скоро истекают', expired: 'С просрочкой', out_of_stock: 'Нет годного остатка', deficit: 'Дефицит' }
export function InventoryPage() {
  const [params, setParams] = useSearchParams()
  const values = readInventorySearch(params)
  const selection = readInventorySelection(params)
  const role = useSelector(selectSession).user?.role
  const canReadSupply = !!role && canAccess(role, sectionForPath('/supplies')!)
  const query = useInventoryQuery(values)
  const data = query.currentData
  const change = (patch: Partial<InventorySearch>) => {
    const next = inventorySearchParams(changeInventorySearch(values, patch))
    inventorySelectionParams(params).forEach((value, key) => next.set(key, value))
    setParams(next)
  }
  const reset = () => setParams(inventorySearchParams(defaultInventorySearch))
  return <div className={styles.page}>
    <header className={styles.row}><div><h1>Склад</h1><p className="text-muted">Годный остаток доступен для работы. Физический включает просроченные партии.</p></div><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить склад</Button></header>
    {canReadSupply && selection.productId && selection.supplyId && <InventorySelection key={`${selection.productId}:${selection.supplyId}`} {...selection} onClear={() => setParams(inventorySearchParams(values))} />}
    <InventoryFilters key={values.search} values={values} onChange={change} />
    <div className={styles.filters} role="group" aria-label="Фильтры склада">
      {inventoryFilters.map((filter) => <Button key={filter} variant={values.filter === filter ? 'primary' : 'secondary'} aria-pressed={values.filter === filter} onClick={() => change({ filter })}>{filterLabels[filter]} · {query.isFetching || query.error ? '—' : data?.counts[filter] ?? '—'}</Button>)}
      <Button variant="ghost" onClick={reset}>Сбросить фильтры</Button>
    </div>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка склада" rows={5} />}
      {!query.isFetching && query.error ? <ErrorState title="Не удалось загрузить склад" onRetry={() => { void query.refetch() }} /> : data && <>
        <p role="status" className="text-caption text-muted">Найдено продуктов: {data.total}. Страница {data.page}.</p>
        {data.items.length === 0 ? data.total > 0 ? <EmptyState title="На этой странице нет продуктов" description="Вернитесь к первой странице списка." /> : values.search || values.category_id || values.filter !== 'all' ? <NoResults query={values.search} onReset={reset} /> : <EmptyState title="Склад пока пуст" description="Продукты появятся после добавления в каталог." /> : <div className={styles.list}>{data.items.map((item) => <InventoryCard key={item.product.id} item={item} />)}</div>}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} onPageChange={(page) => change({ page })} />}
        {data.items.length === 0 && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
