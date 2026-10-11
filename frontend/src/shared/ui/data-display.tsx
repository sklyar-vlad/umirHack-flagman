import { type ReactNode } from 'react'
import { Button, Icon } from './controls'
import { paginationItems } from './pagination'

type Tone = 'neutral' | 'success' | 'warning' | 'danger'

export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`ui-badge ui-badge--${tone}`}>{children}</span>
}

export function Notification({ tone = 'success', title, children, onDismiss }: { tone?: Tone; title: string; children?: ReactNode; onDismiss?: () => void }) {
  return <div className={`ui-alert ui-alert--${tone}`} role={tone === 'danger' ? 'alert' : 'status'}>
    <Icon name={tone === 'danger' || tone === 'warning' ? 'alert' : 'check'} />
    <div className="ui-notification-content"><strong>{title}</strong>{children && <div>{children}</div>}</div>
    {onDismiss && <Button variant="ghost" aria-label={`Скрыть уведомление: ${title}`} onClick={onDismiss}><Icon name="close" /></Button>}
  </div>
}


export function Pagination({ page, pageCount, onPageChange, loading = false, disabled = false }: { page: number; pageCount: number; onPageChange: (page: number) => void; loading?: boolean; disabled?: boolean }) {
  const items = paginationItems(page, pageCount)
  const unavailable = disabled || loading
  return <nav className="ui-pagination" aria-label="Страницы списка" aria-busy={loading || undefined}>
    <Button variant="secondary" disabled={unavailable || page === 1} aria-label="Предыдущая страница" onClick={() => onPageChange(page - 1)}>←</Button>
    {items.map((item) => typeof item === 'number' ? <Button key={item} variant={item === page ? 'primary' : 'secondary'} disabled={unavailable} aria-label={`Страница ${item}`} aria-current={item === page ? 'page' : undefined} onClick={() => { if (item !== page) onPageChange(item) }}>{item}</Button> : <span key={item} aria-hidden="true">…</span>)}
    <Button variant="secondary" disabled={unavailable || page === pageCount} aria-label="Следующая страница" onClick={() => onPageChange(page + 1)}>→</Button>
  </nav>
}

export type DataColumn<T> = { key: string; header: string; render: (row: T) => ReactNode }

export function DataTable<T>({ rows, columns, rowKey, caption, emptyState, loading = false }: { rows: readonly T[]; columns: readonly DataColumn<T>[]; rowKey: (row: T) => string; caption: string; emptyState: ReactNode; loading?: boolean }) {
  if (rows.length === 0) return <div aria-busy={loading || undefined}>{emptyState}</div>
  return <div className="ui-records" aria-busy={loading || undefined}>
    <div className="ui-table-wrap"><table className="ui-table"><caption>{caption}</caption><thead><tr>{columns.map((column) => <th key={column.key} scope="col">{column.header}</th>)}</tr></thead>
      <tbody>{rows.map((row) => <tr key={rowKey(row)}>{columns.map((column) => <td key={column.key}>{column.render(row)}</td>)}</tr>)}</tbody></table></div>
    <section className="ui-mobile-records" aria-label={caption}><ul>{rows.map((row) => <li key={rowKey(row)} className="ui-panel"><dl>{columns.map((column) => <div key={column.key}><dt>{column.header}</dt><dd>{column.render(row)}</dd></div>)}</dl></li>)}</ul></section>
  </div>
}
