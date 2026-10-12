import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { changeDishSearch, defaultDishSearch, dishSearchParams, readDishSearch, useAdminDishesQuery, type DishSearch } from '@/entities/dish'
import { ApiAccessBoundary } from '@/features/access'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { DishFilters } from './DishFilters'
import { RecipeDialog } from './RecipeDialog'
import styles from './AdminDishes.module.css'
const reasons = { ingredient_shortage: 'Недостаточно годного остатка', only_expired_stock: 'Остались только просроченные партии', recipe_missing: 'Рецептура отсутствует', dish_inactive: 'Блюдо отключено' }
export function DishesPage() {
  const [params, setParams] = useSearchParams(), values = readDishSearch(params)
  const [preview, setPreview] = useState<{ id: string; name: string; opener: HTMLElement } | null>(null)
  const query = useAdminDishesQuery(values, { refetchOnMountOrArgChange: true, refetchOnFocus: true, refetchOnReconnect: true }), data = query.currentData
  const change = (patch: Partial<DishSearch>) => setParams(dishSearchParams(changeDishSearch(values, patch)))
  const normalized = dishSearchParams(values), returnTo = `/admin/dishes${normalized.size ? `?${normalized}` : ''}`
  const reset = () => setParams(dishSearchParams(defaultDishSearch))
  return <div className={styles.page}>
    <header className={styles.row}><h1>Каталог блюд и рецептур</h1><div className={styles.actions}><Link className="ui-button ui-button--primary" to={`/admin/dishes/new?${new URLSearchParams({ returnTo })}`}>Создать блюдо</Link><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить каталог</Button></div></header>
    <p className="text-muted">Активные и отключённые блюда, текущие рецептуры и серверная доступность. Доступные порции рассчитаны отдельно для каждого блюда; это не резерв. Сортировка: по названию.</p>
    <DishFilters key={values.search} values={values} onChange={change} onReset={reset} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка каталога блюд" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить каталог блюд" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status">Блюд: {data.total}. Страница {data.page}.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((dish) => <article className={styles.card} key={dish.id} aria-label={`Блюдо: ${dish.name}`}>
          <div className={styles.row}><h2>{dish.name}</h2><span className="text-label">{dish.is_active ? 'Активно' : 'Отключено'}</span></div>
          <p>{dish.description || 'Описание не задано'}</p>
          <dl className={styles.settings}>
            <div><dt>Версия рецептуры</dt><dd>{dish.recipe_version ?? 'Рецептура отсутствует'}</dd></div>
            <div><dt>Ингредиентов</dt><dd>{dish.recipe.length}</dd></div>
            <div><dt>Доступные порции</dt><dd>{dish.available_portions}</dd></div>
            <div><dt>Доступность</dt><dd>{dish.is_available ? 'Доступно' : 'Недоступно'}</dd></div>
          </dl>
          {!!dish.unavailability_reasons.length && <ul>{dish.unavailability_reasons.map((reason) => <li key={reason}>{reasons[reason]}</li>)}</ul>}
          <div className={styles.actions}><Link to={`/admin/dishes/${dish.id}/edit?${new URLSearchParams({ returnTo })}`} aria-label={`Редактировать: ${dish.name}`}>Редактировать</Link><Button variant="secondary" onClick={(event) => setPreview({ id: dish.id, name: dish.name, opener: event.currentTarget })}>Состав: {dish.name}</Button></div>
        </article>)}</div> : values.page > 1 ? <EmptyState title="На этой странице нет блюд" description="Вернитесь к первой странице." /> : values.search || values.is_active !== 'all' || values.availability !== 'all' ? <NoResults query={values.search || 'Выбранные фильтры'} onReset={reset} /> : <EmptyState title="Каталог блюд пуст" description="Создайте первое блюдо и его рецептуру." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
      {preview && <RecipeDialog id={preview.id} name={preview.name} returnFocusTo={preview.opener} onClose={() => setPreview(null)} />}
    </ApiAccessBoundary>
  </div>
}
