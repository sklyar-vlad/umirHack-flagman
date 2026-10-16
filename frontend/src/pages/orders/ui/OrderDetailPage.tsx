import { Link, useParams, useSearchParams } from 'react-router'
import { safeOrderListUrl, useOrderQuery } from '@/entities/order'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError } from '@/shared/api'
import { Button, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { OrderSnapshot } from './OrderSnapshot'
import styles from './OrderDetailPage.module.css'

export function OrderDetailPage() {
  const { id = '' } = useParams(), [params] = useSearchParams()
  const query = useOrderQuery(id, { refetchOnMountOrArgChange: true })
  const status = isApiError(query.error) ? query.error.status : undefined
  const unavailable = status === 404 || status === 422
  const data = query.currentData
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Детали заказа</h1><p className="text-muted">Фактическое списание по партиям.</p></div>
      <Link className="ui-button ui-button--secondary" to={safeOrderListUrl(params.get('returnTo'))}>К истории заказов</Link>
    </header>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка заказа" rows={4} />}
      {unavailable ? <EmptyState title={status === 404 ? 'Заказ не найден' : 'Некорректный адрес заказа'} description={status === 404 ? 'Этот заказ отсутствует. Вернитесь к истории заказов.' : 'Проверьте идентификатор в ссылке или выберите заказ из истории.'} /> : <>
        {query.error && <ErrorState title="Не удалось загрузить заказ" description={data ? 'Показан последний успешный снимок этого заказа. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
        {data && <><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить заказ</Button><OrderSnapshot order={data} loading={query.isFetching} /></>}
      </>}
    </ApiAccessBoundary>
  </section>
}
