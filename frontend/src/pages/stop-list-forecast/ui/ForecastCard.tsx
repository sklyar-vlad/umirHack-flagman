import { Link } from 'react-router'
import type { ForecastRow } from '@/entities/stop-list-forecast'
import { canAccess, sectionForPath, type Role } from '@/entities/session'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge } from '@/shared/ui'
import { assumptionLabel, causeLabels, forecastOutcome } from './forecast-labels'
import styles from './ForecastPage.module.css'

export function ForecastCard({ row, timezone, role }: { row: ForecastRow; timezone: string; role: Role }) {
  const allowed = (path: string) => { const section = sectionForPath(path); return !!section && canAccess(role, section) }
  const dishUrl = allowed('/admin/dishes') ? `/admin/dishes/${row.dish_id}/edit` : allowed('/dishes') ? `/dishes/${row.dish_id}` : null
  return <li className={`ui-panel ${styles.card}`}>
    <h2>{row.dish_name}</h2><Badge tone={row.cause === 'already_unavailable' ? 'danger' : 'warning'}>{causeLabels[row.cause]}</Badge>
    <dl className={styles.facts}>
      <div><dt>{forecastOutcome(row)}</dt><dd>{row.estimated_stop_at === null ? 'Дата не определена' : <time dateTime={row.estimated_stop_at} title={row.estimated_stop_at}>{formatRestaurantDateTime(row.estimated_stop_at, timezone)}</time>}</dd></div>
      <div><dt>Уверенность</dt><dd>{row.confidence === 'low' ? 'Низкая' : 'Недостаточно данных'}</dd></div>
      <div><dt>Ограничивающий продукт</dt><dd>{row.limiting_product_id ?? 'Не определён'}</dd></div>
    </dl>
    {row.confidence === 'insufficient_data' && <p>Истории расхода недостаточно. Оценка по срокам не учитывает неизвестный спрос.</p>}
    {row.estimated_stop_at === null && <p>Отсутствие даты не означает, что блюдо никогда не попадёт в стоп-лист.</p>}
    <details><summary>Допущения модели</summary>{row.assumptions.length ? <ul>{row.assumptions.map((value, index) => <li key={`${index}:${value}`}>{assumptionLabel(value)}</li>)}</ul> : <p>Сервер не указал допущения.</p>}</details>
    <div className="ui-actions">
      {dishUrl && <Link to={dishUrl}>Рецептура блюда</Link>}
      {row.limiting_product_id && allowed('/admin/products') && <Link to={`/admin/products/${row.limiting_product_id}/edit`}>Карточка продукта</Link>}
    </div>
  </li>
}
