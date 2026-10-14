import { isApiError } from '@/shared/api'
import { Link } from 'react-router'
import { useSupplyQuery } from '@/entities/supply'
import { ApiAccessBoundary } from '@/features/access'
import { Button, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { BatchPanel } from './BatchPanel'
import styles from './InventoryPage.module.css'

/** Reads the selected product from its supply even if it is absent from the stock list. */
export function InventorySelection({ productId, supplyId, onClear }: { productId: string; supplyId: string; onClear: () => void }) {
  const query = useSupplyQuery(supplyId, { refetchOnMountOrArgChange: true })
  const errorStatus = isApiError(query.error) ? query.error.status : undefined
  const product = query.currentData?.items.find((line) => line.product.id === productId)?.product
  return <section className={styles.card} aria-label="Выбор из поставки">
    <div className={styles.row}><h2>Партии выбранного продукта</h2><Button variant="ghost" onClick={onClear}>Снять выбор поставки</Button></div>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData ? <Skeleton label="Загрузка выбранного продукта" /> : query.error ? errorStatus === 404
        ? <EmptyState title="Поставка не найдена" description="Снимите выбор и продолжите работу со складом." />
        : <ErrorState title="Не удалось загрузить выбранный продукт" onRetry={() => { void query.refetch() }} />
        : product && query.currentData ? <>
          <p>{product.name} · <Link to={`/supplies/${supplyId}`}>Поставка {query.currentData.number}</Link></p>
          <p className="text-caption text-muted">Открыта вся история продукта, включая израсходованные партии. Партии выбранной поставки отмечены; остальные доступны для сравнения.</p>
          <BatchPanel item={{ product }} supplyId={supplyId} />
        </> : <EmptyState title="Продукт отсутствует в поставке" description="Проверьте ссылку или снимите выбор." />}
    </ApiAccessBoundary>
  </section>
}
