import { useSelector } from 'react-redux'
import { Link, useParams, useSearchParams } from 'react-router'
import { z } from 'zod'
import { safeRecipeListUrl, useDishQuery } from '@/entities/dish'
import type { SessionState } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError } from '@/shared/api'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { RecipeSummary } from './RecipeSummary'
import styles from './Recipes.module.css'

export function RecipeDetailPage() {
  const { id = '' } = useParams(), [params] = useSearchParams(), validId = z.uuid().safeParse(id).success
  const returnTo = safeRecipeListUrl(params.get('returnTo'))
  const isAdmin = useSelector((state: { session: SessionState }) => state.session.user?.role === 'admin')
  const query = useDishQuery(id, { skip: !validId, refetchOnMountOrArgChange: true }), dish = query.currentData
  const missing = !validId || isApiError(query.error) && query.error.status === 404
  return <div className={styles.page}>
    <Link to={returnTo}>← К рецептурам</Link>
    <header className={styles.heading}><h1>{dish && !query.error ? dish.name : 'Рецептура блюда'}</h1>
      {!missing && <Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить состав</Button>}
    </header>
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !dish && <Skeleton label="Загрузка рецептуры" rows={4} />}
      {missing ? <EmptyState title="Блюдо не найдено" description="Вернитесь к списку рецептур и выберите другое блюдо." /> : query.error && <ErrorState title="Не удалось загрузить рецептуру" onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {!query.error && dish && <>
        <section className={styles.card} aria-label="Сведения о блюде">
          {dish.description && <p>{dish.description}</p>}
          <RecipeSummary dish={dish} />
          <p className="text-caption text-muted">Порции рассчитаны отдельно для этого блюда. Общие ингредиенты с другими блюдами не зарезервированы.</p>
          {isAdmin && <Link className="ui-button ui-button--secondary" to={`/admin/dishes/${dish.id}/edit`}>Редактировать рецептуру</Link>}
        </section>
        <section className={styles.card} aria-label="Состав на одну порцию"><h2>Состав на одну порцию</h2>
          {dish.recipe.length ? <ul className={styles.ingredients}>{dish.recipe.map((line) => <li key={line.product_id}><span>{line.product_name}</span><span>{formatQuantity(line.quantity_per_portion, line.unit)}</span></li>)}</ul> : <EmptyState title="Рецептура отсутствует" description="Администратор может добавить состав блюда." />}
        </section>
      </>}
    </ApiAccessBoundary>
  </div>
}
