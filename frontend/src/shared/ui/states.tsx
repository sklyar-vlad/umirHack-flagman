import { type ReactNode } from 'react'
import { Button, Icon } from './controls'

export function Skeleton({ label = 'Загрузка данных', rows = 3 }: { label?: string; rows?: number }) {
  return <div className="ui-skeleton" role="status" aria-label={label} aria-busy="true">{Array.from({ length: Math.max(1, Math.min(20, Math.floor(rows) || 1)) }, (_, index) => <span key={index} aria-hidden="true" />)}</div>
}

export function EmptyState({ title = 'Пока нет данных', description = 'Добавьте первую запись, чтобы начать работу.', action }: { title?: string; description?: string; action?: ReactNode }) {
  return <section className="ui-state"><Icon name="box" /><h2>{title}</h2><p className="text-muted">{description}</p>{action}</section>
}

export function NoResults({ query, onReset }: { query?: string; onReset?: () => void }) {
  return <EmptyState title="Ничего не найдено" description={query ? `По запросу «${query}» нет результатов. Попробуйте изменить запрос или фильтры.` : 'Попробуйте изменить запрос или фильтры.'} action={onReset && <Button variant="secondary" onClick={onReset}>Сбросить фильтры</Button>} />
}

export function ErrorState({ title = 'Не удалось загрузить данные', description = 'Попробуйте ещё раз.', onRetry, retrying = false }: { title?: string; description?: string; onRetry?: () => void; retrying?: boolean }) {
  return <div className="ui-state ui-state-error" role="alert"><Icon name="alert" /><h2>{title}</h2><p>{description}</p>{onRetry && <Button variant="secondary" loading={retrying} loadingLabel="Повторяем…" onClick={onRetry}>Попробовать снова</Button>}</div>
}
