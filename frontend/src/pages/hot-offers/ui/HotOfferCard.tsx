import type { HotOffer } from '@/entities/dish'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button } from '@/shared/ui'
import styles from './HotOffersPage.module.css'
const levels = { high: { label: 'Высокий приоритет', tone: 'danger' }, medium: { label: 'Средний приоритет', tone: 'warning' }, low: { label: 'Низкий приоритет', tone: 'neutral' } } as const
export function HotOfferCard({ item, timezone, disabled, adding, onComposition, onAdd }: { item: HotOffer; timezone: string; disabled: boolean; adding: boolean; onComposition: () => void; onAdd: () => void }) {
  const priority = levels[item.priority_level]
  return <article className={styles.card} aria-label={`Предложение: ${item.dish.name}`}>
    <header className={styles.row}><h3>{item.dish.name}</h3><Badge tone={priority.tone}>{priority.label}</Badge></header>
    <p>Место: {item.rank}. Балл: {item.priority_score.replace('.', ',')}.</p>
    <p>Отдельно доступно: {item.available_portions} порц. Рецептура: {item.dish.recipe_version}.</p>
    <p>{item.reason}</p>
    <h4>Затронутые истекающие партии</h4>
    <ul className={styles.batches}>{item.problem_batches.map((batch) => <li key={batch.batch_id}>
      <strong>{batch.product_name}</strong><p>Поставка №{batch.supply_number}, строка {batch.line_number}.</p>
      <p>Остаток партии: {formatQuantity(batch.remaining_quantity, batch.unit)}. На одну порцию: {formatQuantity(batch.used_for_one_portion, batch.unit)}.</p>
      <p>Годен до: <time dateTime={batch.expires_at}>{formatRestaurantDateTime(batch.expires_at, timezone)}</time>.</p>
    </li>)}</ul>
    <div className={styles.actions}><Button variant="secondary" onClick={onComposition} aria-label={`Состав: ${item.dish.name}`}>Состав</Button>
      <Button disabled={disabled} loading={adding} onClick={onAdd} aria-label={`Добавить в заказ: ${item.dish.name}`}>Добавить в заказ</Button></div>
  </article>
}
