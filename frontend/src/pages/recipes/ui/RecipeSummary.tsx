import type { Dish } from '@/entities/dish'
import { Badge } from '@/shared/ui'
import styles from './Recipes.module.css'
const reasons: Record<Dish['unavailability_reasons'][number], string> = {
  ingredient_shortage: 'Недостаточно годных продуктов', only_expired_stock: 'Остались только просроченные партии',
  recipe_missing: 'Рецептура отсутствует', dish_inactive: 'Блюдо отключено',
}
export function RecipeSummary({ dish }: { dish: Dish }) {
  return <>
    <div className={styles.actions}><Badge>{dish.is_active ? 'Активно' : 'Отключено'}</Badge><Badge>{dish.is_available ? 'Доступно' : 'Недоступно'}</Badge></div>
    <dl className={styles.summary}>
      <div><dt>Версия рецептуры</dt><dd>{dish.recipe_version ?? 'Нет рецептуры'}</dd></div>
      <div><dt>Ингредиентов</dt><dd>{dish.recipe.length}</dd></div>
      <div><dt>Доступные порции</dt><dd>{dish.available_portions}</dd></div>
    </dl>
    {!!dish.unavailability_reasons.length && <ul className={styles.reason}>{dish.unavailability_reasons.map((reason) => <li key={reason}>{reasons[reason]}</li>)}</ul>}
  </>
}
