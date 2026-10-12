import { delay } from 'msw'
import { http } from 'msw/http'
import { readAuditSearch, auditSearchError, type AuditEntry } from '@/entities/audit-log'
import { compareHistoryInstants } from '@/shared/lib/history'
import { createMockError } from './responses'
import { getMockScenario } from './scenarios'

const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
export const mockAuditEntries: AuditEntry[] = Array.from({ length: 65 }, (_, index) => {
  const system = index === 1, settings = index === 2
  return { id: id(900 + index), created_at: new Date(Date.UTC(2026, 9, 10, 9, 0) - index * 60000).toISOString(), actor: system ? null : { id: id(800), name: 'Анна Смирнова' }, action: system ? 'expiration' : 'update', entity_type: system ? 'stock_movement' : settings ? 'settings' : 'product', entity_id: settings ? null : id(system ? 700 : 10), request_id: `audit-demo-${index + 1}`,
    before: system ? { remaining_quantity_after: '1000.000', product_name_snapshot: 'Молоко', unit_snapshot: 'ml' } : settings ? { restaurant_name: 'Бистро №12', revision: 1 } : { name: 'Молоко', target_stock: '3000.000', revision: 1 },
    after: system ? { type: 'expiration', delta_quantity: '-1000.000', remaining_quantity_after: '0.000', product_name_snapshot: 'Молоко', unit_snapshot: 'ml', note: 'Истечение срока годности' } : settings ? { restaurant_name: 'Кладовая', revision: 2 } : { name: 'Молоко', target_stock: '4000.000', revision: 2 },
  }
})
export const auditHandlers = [http.get('/api/v1/audit-log', async ({ request }) => {
  const scenario = getMockScenario()
  if (scenario === 'unauthorized' || scenario === 'session-expired') return createMockError(401, 'UNAUTHORIZED', 'Нет сессии')
  if (scenario === 'forbidden' || scenario === 'audit-forbidden') return createMockError(403, 'FORBIDDEN', 'Нет доступа')
  if (scenario === 'audit-slow') await delay(2000)
  if (scenario === 'audit-error' || scenario === 'server-error') return createMockError(500, 'INTERNAL_ERROR', 'Ошибка чтения')
  if (scenario === 'network-error') return Response.error()
  if (scenario === 'audit-invalid') return Response.json({ items: [{ password_hash: 'not-a-real-secret' }], total: 1, page: 1, page_size: 50, meta: {} })
  const values = readAuditSearch(new URL(request.url).searchParams)
  if (auditSearchError(values)) return createMockError(422, 'VALIDATION_ERROR', 'Проверьте фильтры')
  const items = (scenario === 'audit-empty' ? [] : mockAuditEntries).filter((item) =>
    (!values.actor_id || (values.actor_id === 'system' ? item.actor === null : item.actor?.id === values.actor_id)) &&
    (!values.action || item.action === values.action) && (!values.entity_type || item.entity_type === values.entity_type) &&
    (!values.entity_id || item.entity_id === values.entity_id) &&
    (!values.from || compareHistoryInstants(item.created_at, values.from) >= 0) && (!values.to || compareHistoryInstants(item.created_at, values.to) < 0) &&
    (!values.search || [item.actor?.name ?? 'Система system', item.action, item.entity_type, item.entity_id, item.request_id].some((value) => value?.toLocaleLowerCase().includes(values.search.toLocaleLowerCase()))))
  return Response.json({ items: items.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: items.length, page: values.page, page_size: values.page_size, meta: {} }, { headers: { 'X-Mock-Mode': 'true', 'Cache-Control': 'no-store' } })
})]
