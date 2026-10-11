import { Link, useSearchParams } from 'react-router'
import { changeProductSearch, defaultProductSearch, productSearchParams, readProductSearch, useProductsQuery, type ProductSearch } from '@/entities/product'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { ProductFilters } from './ProductFilters'
import styles from './AdminProducts.module.css'
export function ProductsPage() {
  const [params, setParams] = useSearchParams(), values = readProductSearch(params)
  const query = useProductsQuery(values, { refetchOnMountOrArgChange: true }), data = query.currentData
  const change = (patch: Partial<ProductSearch>) => setParams(productSearchParams(changeProductSearch(values, patch)))
  const returnTo = `/admin/products${params.size ? `?${productSearchParams(values)}` : ''}`
  const reset = () => setParams(productSearchParams(defaultProductSearch))
  return <div className={styles.page}>
    <header className={styles.row}><h1>Каталог продуктов</h1><div className={styles.actions}><Link className="ui-button ui-button--primary" to={`/admin/products/new?${new URLSearchParams({ returnTo })}`}>Создать продукт</Link><Link className="ui-button ui-button--secondary" to="/admin/categories">Управление категориями</Link><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить каталог</Button></div></header>
    <p className="text-muted">Настройки учёта активных и отключённых продуктов. Фактические остатки и партии доступны на складе. Сортировка: по названию.</p>
    <ProductFilters key={values.search} values={values} onChange={change} onReset={reset} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка каталога" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить каталог" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status">Продуктов: {data.total}. Страница {data.page}.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((product) => <article className={styles.card} key={product.id} aria-label={`Продукт: ${product.name}`}>
          <div className={styles.row}><h2>{product.name}</h2><span className="text-label">{product.is_active ? 'Активен' : 'Отключён'}</span></div>
          <dl className={styles.settings}>
            <div><dt>Категория</dt><dd>{product.category.name}</dd></div>
            <div><dt>Базовая единица</dt><dd>{product.unit}</dd></div>
            <div><dt>Единица отображения</dt><dd>{product.display_unit}</dd></div>
            <div><dt>Стандартный срок</dt><dd>{product.standard_shelf_life_hours === null ? 'Не задан' : `${product.standard_shelf_life_hours} ч`}</dd></div>
            <div><dt>Целевой запас</dt><dd>{product.target_stock === null ? 'Не задан' : formatQuantity(product.target_stock, product.unit, product.display_unit)}</dd></div>
          </dl>
          <div className={styles.actions}><Link to={`/admin/products/${product.id}?${new URLSearchParams({ returnTo })}`} aria-label={`Редактировать: ${product.name}`}>Редактировать</Link><Link to={`/inventory?${new URLSearchParams({ search: product.name, product_id: product.id })}`} aria-label={`На склад: ${product.name}`}>На склад</Link></div>
        </article>)}</div> : values.page > 1 ? <EmptyState title="На этой странице нет продуктов" description="Вернитесь к первой странице." /> : values.search || values.category_id || values.is_active !== 'all' ? <NoResults query={values.search || 'Выбранные фильтры'} onReset={reset} /> : <EmptyState title="Каталог пуст" description="Создайте первый продукт после настройки категорий." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
