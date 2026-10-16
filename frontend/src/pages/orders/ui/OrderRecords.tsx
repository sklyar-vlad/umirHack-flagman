import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { orderReasons, type OrderAttempt, type OrderItem, type OrderSummary } from '@/entities/order'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, DataTable } from '@/shared/ui'
import styles from './OrdersPage.module.css'

function Composition({ items }: { items: OrderItem[] }) {
  return <ul className={styles.composition}>{items.map((item, index) => <li key={`${item.dish_id}-${index}`}>{item.dish_name} — {item.portions} порц. <span className="text-caption text-muted">Рецептура: {item.recipe_version ?? 'не указана'}</span></li>)}</ul>
}
function AttemptDetails({ attempt }: { attempt: OrderAttempt }) {
  return <details className={styles.details}><summary>Причина и дефицит</summary>
    <p>{orderReasons[attempt.reason_code]}</p>
    {attempt.shortages.length > 0 && <ul>{attempt.shortages.map((item, index) => <li key={`${item.product_id}-${index}`}><strong>{item.product_name}</strong><dl>
      <div><dt>Требуется</dt><dd>{formatQuantity(item.required_quantity, item.unit)}</dd></div>
      <div><dt>Доступно</dt><dd>{formatQuantity(item.available_quantity, item.unit)}</dd></div>
      <div><dt>Не хватает</dt><dd>{formatQuantity(item.missing_quantity, item.unit)}</dd></div>
      <div><dt>Просрочено</dt><dd>{formatQuantity(item.expired_quantity, item.unit)}</dd></div>
    </dl><p>{item.reason_code === 'only_expired_stock' ? 'Весь физический остаток просрочен' : 'Недостаточно пригодного остатка'}</p></li>)}</ul>}
    {attempt.issues.length > 0 && <ul>{attempt.issues.map((issue, index) => <li key={`${issue.dish_id}-${index}`}><strong>{attempt.items.find((item) => item.dish_id === issue.dish_id)?.dish_name ?? `Блюдо ${issue.dish_id}`}</strong>: {issue.message}{issue.current_recipe_version !== null && <p>Версия рецептуры на момент отказа: {issue.current_recipe_version}</p>}</li>)}</ul>}
    {!attempt.shortages.length && !attempt.issues.length && <p>Дополнительные сведения в сохранённой попытке отсутствуют.</p>}
    <p className="text-caption text-muted">Снимок на момент отказа, не текущий остаток.</p>
  </details>
}
interface Common { timezone: string; returnTo: string; loading: boolean; emptyState: ReactNode }
export function OrderRecords({ items, timezone, returnTo, loading, emptyState }: Common & { items: OrderSummary[] }) {
  return <DataTable rows={items} rowKey={(item) => item.id} caption="История успешных заказов" loading={loading} emptyState={emptyState} columns={[
    { key: 'number', header: 'Номер', render: (item) => <Link to={`/orders/${item.id}?returnTo=${encodeURIComponent(returnTo)}`} aria-label={`Заказ ${item.number}`}>{item.number}</Link> },
    { key: 'date', header: 'Создан', render: (item) => <time dateTime={item.created_at}>{formatRestaurantDateTime(item.created_at, timezone)}</time> },
    { key: 'items', header: 'Состав', render: (item) => <Composition items={item.items} /> },
    { key: 'portions', header: 'Всего порций', render: (item) => item.total_portions },
    { key: 'actor', header: 'Сотрудник', render: (item) => item.created_by.name },
    { key: 'status', header: 'Статус', render: () => <Badge tone="success">Выполнен</Badge> },
  ]} />
}
export function AttemptRecords({ items, timezone, loading, emptyState }: Common & { items: OrderAttempt[] }) {
  return <DataTable rows={items} rowKey={(item) => item.id} caption="История отклонённых попыток" loading={loading} emptyState={emptyState} columns={[
    { key: 'id', header: 'ID попытки', render: (item) => <span className={styles.attemptId}>{item.id}</span> },
    { key: 'date', header: 'Создана', render: (item) => <time dateTime={item.created_at}>{formatRestaurantDateTime(item.created_at, timezone)}</time> },
    { key: 'items', header: 'Состав', render: (item) => <Composition items={item.items} /> },
    { key: 'actor', header: 'Сотрудник', render: (item) => item.created_by.name },
    { key: 'reason', header: 'Отказ', render: (item) => <><Badge tone="warning">{orderReasons[item.reason_code]}</Badge><AttemptDetails attempt={item} /></> },
  ]} />
}
