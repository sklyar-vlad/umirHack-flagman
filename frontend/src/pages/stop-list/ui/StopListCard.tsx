import { useSelector } from 'react-redux'
import { Link } from 'react-router'
import type { Dish } from '@/entities/dish'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { formatQuantity } from '@/shared/lib/quantity'
import { Badge } from '@/shared/ui'
import styles from './StopListPage.module.css'

const reasons = { ingredient_shortage: 'Недостаточно годного остатка', only_expired_stock: 'Остались только просроченные партии', recipe_missing: 'Рецептура отсутствует', dish_inactive: 'Блюдо отключено' }
export function StopListCard({ dish }: { dish: Dish }) {
  const role = useSelector(selectSession).user?.role
  const allowed = (path: string) => !!role && canAccess(role, sectionForPath(path)!)
  return <article className={styles.card} aria-label={`Блюдо: ${dish.name}`}>
    <div className={styles.row}><h2>{dish.name}</h2><Badge tone="danger">Недоступно</Badge></div>
    {dish.description && <p className="text-muted">{dish.description}</p>}
    <div className={styles.reasons}>{dish.unavailability_reasons.map((reason) => <Badge tone="warning" key={reason}>{reasons[reason]}</Badge>)}</div>
    <p className="text-caption text-muted">Доступно порций: {dish.available_portions}. Потребность рассчитана на одну порцию.</p>
    <details><summary>Состав и причины: {dish.name}</summary>
      <div className={styles.details}>
        {dish.recipe_version === null ? <div><p>Без рецептуры нельзя рассчитать потребность и доступность. Обратитесь к администратору.</p>
          {role === 'admin' && <Link to={`/dishes/${dish.id}`}>Добавить рецептуру: {dish.name}</Link>}</div> : <>
          <h3>Состав на одну порцию · рецепт {dish.recipe_version}</h3>
          <ul className={styles.ingredients}>{dish.recipe.map((ingredient) => <li key={ingredient.product_id}><span>{ingredient.product_name} — {formatQuantity(ingredient.quantity_per_portion, ingredient.unit)}</span>
            {allowed('/inventory') && <Link to={`/inventory?${new URLSearchParams({ search: ingredient.product_name, product_id: ingredient.product_id })}`}>На склад: {ingredient.product_name}</Link>}</li>)}</ul>
          <h3>Дефицит на одну порцию</h3>
          {dish.shortages.map((shortage) => <section key={shortage.product_id} className={styles.shortage} aria-label={`Дефицит: ${shortage.product_name}`}>
            <strong>{shortage.product_name}</strong><p>{reasons[shortage.reason_code]}</p>
            <dl className={styles.quantities}>
              <div><dt>Требуется</dt><dd>{formatQuantity(shortage.required_quantity, shortage.unit)}</dd></div>
              <div><dt>Доступно годного</dt><dd>{formatQuantity(shortage.available_quantity, shortage.unit)}</dd></div>
              <div><dt>Недостаёт</dt><dd>{formatQuantity(shortage.missing_quantity, shortage.unit)}</dd></div>
              <div><dt>Просрочено</dt><dd>{formatQuantity(shortage.expired_quantity, shortage.unit)}</dd></div>
            </dl>
          </section>)}
          {allowed('/recommendations/procurement') && <Link to="/recommendations/procurement">Рекомендации по закупкам: {dish.name}</Link>}
        </>}
      </div>
    </details>
  </article>
}
