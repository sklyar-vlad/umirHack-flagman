import { useSelector } from 'react-redux'
import { Link } from 'react-router'
import { useDashboardQuery } from '@/entities/dashboard'
import { canAccess, sections, selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import styles from './DashboardPage.module.css'

const priorities = { high: 'Высокий приоритет', medium: 'Средний приоритет', low: 'Низкий приоритет' }
export function DashboardPage() {
  const session = useSelector(selectSession)
  const allowed = (id: string) => !!session.user && canAccess(session.user.role, sections.find((section) => section.id === id)!)
  const query = useDashboardQuery(undefined, { refetchOnMountOrArgChange: true })
  const data = query.currentData
  return <div className={styles.page}>
    <header className={styles.row}><div><h1>Обзор</h1><p className="text-muted">Состояние склада и рекомендации на один момент времени.</p></div><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить обзор</Button></header>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка обзора" rows={5} />}
      {query.error && <ErrorState title="Не удалось загрузить обзор" description={data ? 'Показан последний успешный снимок. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status" className="text-caption text-muted">{data.settings.restaurant_name}. Снимок: <time dateTime={data.meta.server_time}>{formatRestaurantDateTime(data.meta.server_time, data.settings.timezone)}</time>. Часовой пояс: {data.settings.timezone}. Скоро истекает: в течение {data.settings.expiring_threshold_hours} ч. Снимок обновляется при серверных изменениях и восстановлении связи.</p>
        <div className={styles.metrics}>
          {[
            { label: 'Истекающие партии', count: data.expiring_batches_count, to: '/inventory?filter=expiring&sort=nearest_expires_at' },
            { label: 'Просроченные партии', count: data.expired_batches_count, to: '/inventory?filter=expired' },
            { label: 'Блюда в стоп-листе', count: data.stop_list_dishes_count, to: allowed('stop-list') ? '/stop-list' : null },
            { label: 'Продукты без годного остатка', count: data.out_of_stock_products_count, to: '/inventory?filter=out_of_stock' },
          ].map((metric) => <section key={metric.label} className={styles.card} aria-label={metric.label}><h2>{metric.label}</h2><p className={styles.metric}>{metric.count}</p>{metric.to && <Link to={metric.to}>Открыть: {metric.label.toLocaleLowerCase('ru')}</Link>}</section>)}
        </div>
        <section className={styles.block} aria-labelledby="dashboard-attention"><header className={styles.row}><h2 id="dashboard-attention">Требуют внимания</h2><Link to="/inventory">Весь склад</Link></header>
          <p>Всего проблемных партий: {data.attention_total}. Показано: {data.attention_batches.length}.</p>
          <p className="text-caption text-muted">Счётчики относятся к партиям, складские фильтры — к продуктам. Один продукт может иметь несколько проблемных партий.</p>
          {data.attention_batches.length ? <div className={styles.attention}>{data.attention_batches.map((batch) => <article key={batch.batch_id} className={styles.card} aria-label={`Партия: ${batch.product_name}, строка ${batch.line_number}`}>
            <header className={styles.row}><h3>{batch.product_name}</h3><Badge tone={batch.status === 'expired' ? 'danger' : 'warning'}>{batch.status === 'expired' ? 'Просрочена' : 'Скоро истекает'}</Badge></header>
            <p>Остаток: {formatQuantity(batch.remaining_quantity, batch.unit)}. Поставка №{batch.supply_number}, строка {batch.line_number}.</p>
            <p>Годен до: <time dateTime={batch.expires_at}>{formatRestaurantDateTime(batch.expires_at, data.settings.timezone)}</time>.</p>
            <Link to={`/inventory?${new URLSearchParams({ filter: batch.status, search: batch.product_name, product_id: batch.product_id, ...(allowed('supplies') && { supply_id: batch.supply_id }) })}`}>На склад: {batch.product_name}, строка {batch.line_number}</Link>
          </article>)}</div> : <EmptyState title="Нет проблемных партий" description="По этому снимку нет истекающих или просроченных партий с положительным остатком." />}
        </section>
        <div className={styles.recommendations}>
          {allowed('offers') && <section className={styles.block} aria-labelledby="dashboard-offers"><header className={styles.row}><h2 id="dashboard-offers">Продвигать блюда</h2><Link to="/recommendations/offers">Все горячие предложения</Link></header>
            <p className="text-caption text-muted">Первые предложения в порядке сервера. Доступность каждого блюда отдельно не гарантирует всю корзину и не резервирует продукты.</p>
            {data.hot_offers.length ? data.hot_offers.map((offer) => <article key={offer.dish.id} className={styles.card} aria-label={`Предложение: ${offer.dish.name}`}><h3>{offer.dish.name}</h3><p>Место: {offer.rank}. {priorities[offer.priority_level]}. Балл: {offer.priority_score.replace('.', ',')}.</p><p>Отдельно доступно: {offer.available_portions} порц.</p><p>{offer.reason}</p><Link to={`/recommendations/offers?${new URLSearchParams({ search: offer.dish.name })}`}>Предложение: {offer.dish.name}</Link></article>) : <EmptyState title="Нет горячих предложений" description="Сейчас сервер не рекомендует блюда для продвижения." />}
          </section>}
          {allowed('procurement') && <section className={styles.block} aria-labelledby="dashboard-procurement"><header className={styles.row}><h2 id="dashboard-procurement">Закупить продукты</h2><Link to="/recommendations/procurement">Все рекомендации закупок</Link></header>
            <p className="text-caption text-muted">Первые рекомендации в порядке сервера. Продукты без целевого запаса не включены; это не полный список отсутствующих продуктов.</p>
            {data.procurement_recommendations.length ? data.procurement_recommendations.map((item) => <article key={item.product_id} className={styles.card} aria-label={`Закупить: ${item.product_name}`}><h3>{item.product_name}</h3><p>К закупке: {formatQuantity(item.recommended_quantity, item.unit, item.display_unit)}. Годный остаток: {formatQuantity(item.available_quantity, item.unit, item.display_unit)}. Цель: {formatQuantity(item.target_stock, item.unit, item.display_unit)}.</p><p>{item.reason}</p><Link to={`/recommendations/procurement?${new URLSearchParams({ search: item.product_name })}`}>Закупка: {item.product_name}</Link></article>) : <EmptyState title="Закупки не требуются" description="Сервер не вернул положительных рекомендаций для продуктов с целевым запасом." />}
          </section>}
        </div>
      </>}
    </ApiAccessBoundary>
    {import.meta.env.DEV && <nav aria-label="Проверки приложения" className={styles.row}>
      <Link to="/dev/access">Проверить доступ и переходы</Link>
      <Link to="/dev/api">Проверить подключение к API</Link><Link to="/dev/ui">Компоненты интерфейса</Link>
    </nav>}
  </div>
}
