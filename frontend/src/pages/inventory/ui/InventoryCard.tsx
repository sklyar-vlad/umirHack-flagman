import { useState } from 'react'
import { useSelector } from 'react-redux'
import { selectSession } from '@/entities/session'
import type { InventoryItem } from '@/entities/inventory'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Badge, Button, Icon } from '@/shared/ui'
import { BatchPanel } from './BatchPanel'
import { productStatuses, remainingTime } from './inventory-format'
import styles from './InventoryPage.module.css'

export function InventoryCard({ item }: { item: InventoryItem }) {
  const [expanded, setExpanded] = useState(false)
  const timezone = useSelector(selectSession).settings?.timezone
  const { product } = item
  const quantity = (value: string | null) => formatQuantity(value, product.unit, product.display_unit)
  return <article className={styles.card} aria-label={product.name}>
    <div className={styles.row}><div><h2>{product.name}</h2><p className="text-caption text-muted">{product.category.name} · {quantity('1.000').split(' ').slice(1).join(' ')}{!product.is_active && ' · Продукт отключён'}</p></div>
      <Badge tone={item.status === 'expired' ? 'danger' : item.status === 'expiring' ? 'warning' : item.status === 'normal' ? 'success' : 'neutral'}>{productStatuses[item.status]}</Badge>
    </div>
    <dl className={styles.metrics}>
      <div><dt>Годный остаток</dt><dd className="text-strong">{quantity(item.available_quantity)}</dd></div>
      <div><dt>Физический остаток</dt><dd>{quantity(item.physical_quantity)}</dd></div>
      <div><dt>Скоро истекает</dt><dd>{quantity(item.expiring_quantity)}</dd></div>
      <div><dt>Просрочено</dt><dd>{quantity(item.expired_quantity)}</dd></div>
      <div><dt>Партий с остатком</dt><dd>{item.batches_count}</dd></div>
      <div><dt>Ближайший годный срок</dt><dd>{item.nearest_expires_at === null ? 'Нет годных партий' : formatRestaurantDateTime(item.nearest_expires_at, timezone)}{item.remaining_seconds !== null && <small>{remainingTime(item.remaining_seconds)}</small>}</dd></div>
      <div><dt>Целевой запас</dt><dd>{item.target_stock === null ? 'Не задан' : quantity(item.target_stock)}</dd></div>
      <div><dt>Дефицит</dt><dd>{item.deficit_quantity === null ? 'Не рассчитан' : quantity(item.deficit_quantity)}</dd></div>
    </dl>
    <div className={styles.actions}>
      {item.is_out_of_stock && item.status !== 'out_of_stock' && <Badge>Нет годного остатка</Badge>}
      {item.has_expiring && item.status !== 'expiring' && <Badge tone="warning">Скоро истекает</Badge>}
      <Button variant="secondary" aria-expanded={expanded} aria-controls={`batches-${product.id}`} onClick={() => setExpanded(!expanded)}><Icon name="chevron" />{expanded ? 'Скрыть партии' : 'Раскрыть партии'} {product.name}</Button>
    </div>
    <div id={`batches-${product.id}`}>{expanded && <BatchPanel item={item} />}</div>
  </article>
}
