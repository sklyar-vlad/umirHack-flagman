import { Link } from 'react-router'
import { auditActions, auditEntities, type AuditEntry } from '@/entities/audit-log'
import { Dialog } from '@/shared/ui'
import styles from './AuditPage.module.css'

const fieldNames: Record<string, string> = { name: 'Название', email: 'Email', role: 'Роль', is_active: 'Активен', revision: 'Ревизия', category_id: 'ID категории', unit: 'Базовая единица', display_unit: 'Единица отображения', standard_shelf_life_hours: 'Стандартный срок (часы)', target_stock: 'Целевой запас (базовая единица)', description: 'Описание', recipe: 'Рецептура', recipe_version: 'Версия рецепта', recipe_version_id: 'ID версии рецепта', product_id: 'ID продукта', quantity_per_portion: 'На порцию (базовая единица)', number: 'Номер', received_at: 'Дата приёмки', batches: 'Партии', batch_id: 'ID партии', line_number: 'Строка', initial_quantity: 'Исходное количество', remaining_quantity: 'Остаток', produced_at: 'Дата производства', shelf_life_hours: 'Срок (часы)', expiry_source: 'Способ расчёта срока', expires_at: 'Годен до', items: 'Позиции', dish_id: 'ID блюда', dish_name_snapshot: 'Название блюда', portions: 'Порции', order_item_id: 'ID позиции заказа', type: 'Тип движения', delta_quantity: 'Изменение количества', remaining_quantity_after: 'Остаток после движения', product_name_snapshot: 'Продукт', unit_snapshot: 'Единица', note: 'Причина', restaurant_name: 'Ресторан', timezone: 'Часовой пояс', expiring_threshold_hours: 'Порог истечения (часы)', auto_writeoff_expired: 'Автосписание' }
function flatten(value: unknown, prefix = '', result: Record<string, string> = {}) {
  if (value !== null && typeof value === 'object') {
    const entries = Object.entries(value)
    if (!entries.length && prefix) result[prefix] = Array.isArray(value) ? 'Пустой список' : 'Нет полей'
    for (const [key, child] of entries) flatten(child, [prefix, Array.isArray(value) ? `Строка ${Number(key) + 1}` : fieldNames[key] ?? key].filter(Boolean).join(' · '), result)
  } else if (prefix) result[prefix] = value === null ? 'Не задано' : typeof value === 'boolean' ? value ? 'Да' : 'Нет' : String(value)
  return result
}
function auditObjectPath(entry: AuditEntry): string | null {
  if (entry.entity_type === 'settings') return '/admin/settings'
  if (!entry.entity_id) return null
  const routes = { product: '/admin/products', dish: '/admin/dishes', user: '/admin/users', supply: '/supplies', order: '/orders' }
  return entry.entity_type in routes ? `${routes[entry.entity_type as keyof typeof routes]}/${entry.entity_id}` : null
}
export function AuditDetails({ entry, onClose }: { entry: AuditEntry; onClose: () => void }) {
  const before = flatten(entry.before), after = flatten(entry.after), fields = [...new Set([...Object.keys(before), ...Object.keys(after)])]
  const path = auditObjectPath(entry)
  return <Dialog open title="Изменения записи" onClose={onClose} description={`${auditActions[entry.action]} · ${auditEntities[entry.entity_type]}`}>
    <dl className={styles.identity}><div><dt>Автор</dt><dd>{entry.actor?.name ?? 'Система'}</dd></div><div><dt>ID записи</dt><dd>{entry.id}</dd></div><div><dt>ID объекта</dt><dd>{entry.entity_id ?? (entry.entity_type === 'settings' ? 'Единственные настройки ресторана' : 'Не указан')}</dd></div><div><dt>Request ID</dt><dd>{entry.request_id ?? 'Не указан'}</dd></div></dl>
    {entry.action === 'password_change' && <p>Зафиксирован факт смены пароля. Пароль и его хеш не доступны для просмотра.</p>}
    {fields.length ? <div className={styles.changes}>{fields.map((field) => <section key={field} className={styles.change}><h3>{field}{before[field] !== after[field] ? ' · Изменено' : ''}</h3><dl><div><dt>До</dt><dd>{before[field] ?? 'Поле отсутствовало'}</dd></div><div><dt>После</dt><dd>{after[field] ?? 'Поле отсутствует'}</dd></div></dl></section>)}</div> : <p>Изменения полей для этой записи не сохранены.</p>}
    {path ? <><Link className="ui-button ui-button--secondary" to={path}>Открыть текущий объект</Link><p className="text-caption text-muted">Откроется текущее состояние. Если объект недоступен, его страница покажет причину; историческая запись останется в журнале.</p></> : <p className="text-caption text-muted">Отдельная страница этого объекта недоступна. Используйте ID для поиска в журнале.</p>}
  </Dialog>
}
