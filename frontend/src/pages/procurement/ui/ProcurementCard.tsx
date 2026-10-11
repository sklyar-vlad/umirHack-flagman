import { useSelector } from 'react-redux'
import { Link } from 'react-router'
import type { ProcurementItem } from '@/entities/procurement'
import { canAccess, sectionForPath, selectSession } from '@/entities/session'
import { formatQuantity } from '@/shared/lib/quantity'
import { Badge } from '@/shared/ui'
import styles from './ProcurementPage.module.css'
const reasons = { below_target: 'Ниже целевого запаса', no_valid_stock: 'Нет годного остатка', only_expired_stock: 'Остались только просроченные партии' }
export function ProcurementCard({ item }: { item: ProcurementItem }) {
  const role = useSelector(selectSession).user?.role
  const allowed = (path: string) => !!role && canAccess(role, sectionForPath(path)!)
  const quantity = (raw: string) => formatQuantity(raw, item.unit, item.display_unit)
  return <article className={styles.card} aria-label={`Закупить: ${item.product_name}`}>
    <div className={styles.row}><h3>{item.product_name}</h3><Badge tone="warning">К закупке: {quantity(item.recommended_quantity)}</Badge></div>
    <dl className={styles.quantities}>
      <div><dt>Годный остаток</dt><dd>{quantity(item.available_quantity)}</dd></div>
      <div><dt>Целевой запас</dt><dd>{quantity(item.target_stock)}</dd></div>
      <div><dt>Докупить</dt><dd>{quantity(item.recommended_quantity)}</dd></div>
      <div><dt>Просрочено</dt><dd>{quantity(item.expired_quantity)}</dd></div>
    </dl>
    <div className={styles.actions}>{item.reason_codes.map((code) => <Badge tone="warning" key={code}>{reasons[code]}</Badge>)}</div>
    <p>{item.reason}</p>
    <div><h4>Блокируемые блюда</h4>{item.affected_dishes.length ? <ul>{item.affected_dishes.map((dish) => <li key={dish.id}>{allowed('/stop-list') ? <Link to={`/stop-list?${new URLSearchParams({ search: dish.name })}`}>{dish.name}</Link> : dish.name}</li>)}</ul> : <p className="text-muted">Нет блюд с дефицитом этого ингредиента на одну порцию. Закупка нужна для целевого запаса.</p>}</div>
    <div className={styles.actions}>
      {allowed('/inventory') && <Link to={`/inventory?${new URLSearchParams({ search: item.product_name, product_id: item.product_id })}`}>На склад: {item.product_name}</Link>}
      {allowed('/supplies') && <Link to={`/supplies/new?${new URLSearchParams({ product_id: item.product_id })}`}>Принять поставку: {item.product_name}</Link>}
    </div>
  </article>
}
