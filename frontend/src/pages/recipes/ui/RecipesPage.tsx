import { useSelector } from 'react-redux'
import { Link, useSearchParams } from 'react-router'
import { changeDishSearch, defaultDishSearch, dishSearchParams, readDishSearch, useDishCatalogueQuery, type DishSearch } from '@/entities/dish'
import type { SessionState } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { DishFilters } from '@/widgets/dish-filters'
import { RecipeSummary } from './RecipeSummary'
import styles from './Recipes.module.css'

export function RecipesPage() {
  const [params, setParams] = useSearchParams(), values = readDishSearch(params)
  const isAdmin = useSelector((state: { session: SessionState }) => state.session.user?.role === 'admin')
  const query = useDishCatalogueQuery(values, { refetchOnMountOrArgChange: true }), data = query.currentData
  const change = (patch: Partial<DishSearch>) => setParams(dishSearchParams(changeDishSearch(values, patch)))
  const reset = () => setParams(dishSearchParams(defaultDishSearch))
  const normalized = dishSearchParams(values), returnTo = `/dishes${normalized.size ? `?${normalized}` : ''}`
  return <div className={styles.page}>
    <header className={styles.heading}><h1>Рецептуры</h1><div className={styles.actions}>
      {isAdmin && <Link className="ui-button ui-button--secondary" to="/admin/dishes">Управлять рецептурами</Link>}
      <Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить рецептуры</Button>
    </div></header>
    <p className="text-muted">Состав и расход продуктов на одну порцию. Доступность рассчитана по годным остаткам; просмотр не резервирует продукты.</p>
    <DishFilters key={values.search} values={values} onChange={change} onReset={reset} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !data && <Skeleton label="Загрузка рецептур" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить рецептуры" description={data ? 'Показаны последние загруженные данные. Они могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <>
        <p role="status">Блюд: {data.total}. Страница {data.page}.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((dish) => <article className={styles.card} key={dish.id} aria-label={`Рецептура: ${dish.name}`}>
          <h2><Link to={`/dishes/${dish.id}?${new URLSearchParams({ returnTo })}`}>{dish.name}</Link></h2>
          {dish.description && <p className="text-muted">{dish.description}</p>}
          <RecipeSummary dish={dish} />
          {dish.recipe.length ? <ul className={styles.ingredients} aria-label={`Состав на порцию: ${dish.name}`}>
            {dish.recipe.slice(0, 3).map((line) => <li key={line.product_id}><span>{line.product_name}</span><span>{formatQuantity(line.quantity_per_portion, line.unit)}</span></li>)}
          </ul> : <p>Состав пока не задан.</p>}
          <Link to={`/dishes/${dish.id}?${new URLSearchParams({ returnTo })}`}>Открыть рецептуру{dish.recipe.length > 3 ? ` · ещё ${dish.recipe.length - 3} ингредиентов` : ''}</Link>
        </article>)}</div> : values.page > 1 ? <EmptyState title="На этой странице нет рецептур" description="Вернитесь к первой странице." /> : values.search || values.is_active !== 'all' || values.availability !== 'all' ? <NoResults query={values.search || 'Выбранные фильтры'} onReset={reset} /> : <EmptyState title="Рецептур пока нет" description={isAdmin ? 'Добавьте первое блюдо и его состав в разделе управления рецептурами.' : 'Администратор может добавить блюда и их состав.'} />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
