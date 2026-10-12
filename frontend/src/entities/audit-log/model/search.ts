import { z } from 'zod'
import { defaultHistorySearch, historyPeriodError, historySearchParams, readHistorySearch, type HistorySearch } from '@/shared/lib/history'
import { auditActionSchema, auditEntitySchema } from './contracts'

export interface AuditSearch extends HistorySearch { actor_id: string; action: string; entity_type: string; entity_id: string }
export const defaultAuditSearch: AuditSearch = { ...defaultHistorySearch, actor_id: '', action: '', entity_type: '', entity_id: '' }
export function readAuditSearch(params: URLSearchParams): AuditSearch {
  return { ...readHistorySearch(params), actor_id: params.get('actor_id') ?? '', action: params.get('action') ?? '', entity_type: params.get('entity_type') ?? '', entity_id: params.get('entity_id') ?? '' }
}
export function auditSearchError(values: AuditSearch): string | null {
  return historyPeriodError(values) || (values.actor_id && values.actor_id !== 'system' && !z.uuid().safeParse(values.actor_id).success ? 'Автор: укажите UUID пользователя или system.' : null)
    || (values.entity_id && !z.uuid().safeParse(values.entity_id).success ? 'ID объекта должен быть UUID.' : null)
    || (values.action && !auditActionSchema.safeParse(values.action).success ? 'Некорректное действие в адресе.' : null)
    || (values.entity_type && !auditEntitySchema.safeParse(values.entity_type).success ? 'Некорректный тип объекта в адресе.' : null)
}
export function auditSearchParams(values: AuditSearch): URLSearchParams {
  const params = historySearchParams(values)
  for (const key of ['actor_id', 'action', 'entity_type', 'entity_id'] as const) if (values[key]) params.set(key, values[key])
  return params
}
export function changeAuditSearch(values: AuditSearch, patch: Partial<AuditSearch>): AuditSearch { return { ...values, ...patch, page: patch.page ?? 1 } }
