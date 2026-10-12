import type { ErrorDetails, ErrorResponse, HealthcheckResponse } from '@/shared/api'
import type { MockScenario } from './scenarios'

function mockHeaders(requestId: string) {
  return { 'X-Request-ID': requestId, 'X-Mock-Mode': 'true' }
}

export function createMockError(
  status: number,
  code: string,
  message: string,
  details: ErrorDetails = {},
  retryAfter?: string,
): Response {
  const requestId = crypto.randomUUID()
  const body: ErrorResponse = {
    error: { code, message, details },
    request_id: requestId,
  }
  return Response.json(body, {
    status,
    headers: {
      ...mockHeaders(requestId),
      ...(retryAfter ? { 'Retry-After': retryAfter } : {}),
    },
  })
}

/** Ошибки здесь намеренно форсируются для проверки транспорта, не прав healthcheck. */
export function createHealthcheckResponse(scenario: MockScenario): Response {
  switch (scenario) {
    case 'movements-empty':
    case 'movements-slow':
    case 'movements-error':
    case 'movements-forbidden':
    case 'movements-invalid':
    case 'audit-empty':
    case 'audit-slow':
    case 'audit-error':
    case 'audit-forbidden':
    case 'audit-invalid':
    case 'settings-read-slow':
    case 'settings-read-error':
    case 'settings-save-slow':
    case 'settings-revision-conflict':
    case 'settings-validation':
    case 'settings-feature-unavailable':
    case 'settings-lost-response':
    case 'user-save-slow':
    case 'user-email-conflict':
    case 'user-last-admin':
    case 'user-revision-conflict':
    case 'user-validation':
    case 'user-lost-response':
    case 'user-not-found':
    case 'users-empty':
    case 'users-error':
    case 'users-slow':
    case 'roles-error':
    case 'roles-slow':
    case 'roles-forbidden':
    case 'dish-preview-slow':
    case 'dish-preview-error':
    case 'dish-save-slow':
    case 'dish-revision-conflict':
    case 'dish-validation':
    case 'dish-duplicate':
    case 'dish-lost-response':
    case 'dish-catalog-new-version':
    case 'product-save-slow':
    case 'product-lost-response':
    case 'product-revision-conflict':
    case 'product-validation-error':
    case 'product-unit-locked':
    case 'product-in-use':
    case 'product-not-found':
    case 'category-validation-error':
    case 'category-not-found':
    case 'categories-empty':
    case 'category-save-slow':
    case 'category-revision-conflict':
    case 'category-lost-response':
    case 'dashboard-empty':
    case 'dashboard-preview':
    case 'dashboard-expired':
    case 'hot-offers-empty':
    case 'hot-offers-expired':
    case 'hot-offers-expiring':
    case 'hot-offers-high':
    case 'dish-detail-error':
    case 'dish-detail-slow':
    case 'dish-detail-missing':
    case 'procurement-empty':
    case 'procurement-expired':
    case 'stop-list-empty':
    case 'stop-list-expired':
    case 'stop-list-recipe':
    case 'order-detail-demo':
    case 'order-detail-split':
    case 'order-not-found':
    case 'dishes-empty':
    case 'dishes-error':
    case 'dishes-slow':
    case 'order-check-slow':
    case 'order-check-error':
    case 'order-check-shortage':
    case 'order-submit-slow':
    case 'order-network':
    case 'order-lost-response':
    case 'order-shortage':
    case 'order-recipe-changed':
    case 'order-inactive':
    case 'order-final-recipe':
    case 'order-final-inactive':
    case 'order-validation':
    case 'order-rate-limit':
    case 'products-empty':
    case 'products-error':
    case 'products-slow':
    case 'reception-slow':
    case 'reception-network':
    case 'reception-lost-response':
    case 'reception-validation':
    case 'reception-conflict':
    case 'reception-in-progress':
    case 'supply-not-found':
    case 'orders-empty':
    case 'supplies-empty':
    case 'supplies-received':
    case 'inventory-empty':
    case 'batches-empty':
    case 'batches-error':
    case 'batches-slow':
    case 'categories-error':
    case 'role-admin':
    case 'role-storekeeper':
    case 'role-order-operator':
    case 'invalid-password':
    case 'disabled-user':
    case 'session-expired':
    case 'success':
    case 'slow': {
      const body: HealthcheckResponse = { status: 'ok', service: 'umirHack-flagman' }
      return Response.json(body, { headers: mockHeaders(crypto.randomUUID()) })
    }
    case 'network-error': return Response.error()
    case 'unauthorized': return createMockError(401, 'UNAUTHORIZED', 'Сессия отсутствует или истекла')
    case 'forbidden': return createMockError(403, 'FORBIDDEN', 'Недостаточно прав')
    case 'conflict': return createMockError(409, 'CONFLICT', 'Состояние ресурса изменилось')
    case 'validation-error':
      return createMockError(422, 'VALIDATION_ERROR', 'Некорректные данные', {
        fields: [{ path: 'page', code: 'invalid_value', message: 'Ожидается целое число больше нуля' }],
      })
    case 'rate-limit': return createMockError(429, 'RATE_LIMIT_EXCEEDED', 'Слишком много запросов', {}, '10')
    case 'server-error': return createMockError(500, 'INTERNAL_ERROR', 'Внутренняя ошибка сервера')
    case 'bad-gateway':
      return new Response('<html><body><h1>502 Bad Gateway</h1></body></html>', {
        status: 502,
        headers: { ...mockHeaders(crypto.randomUUID()), 'Content-Type': 'text/html', 'Retry-After': '10' },
      })
    case 'invalid-json':
      return new Response('{ invalid json', {
        headers: { ...mockHeaders(crypto.randomUUID()), 'Content-Type': 'application/json' },
      })
    case 'invalid-schema':
      return Response.json({ status: 'ok', service: 'unexpected-service' }, {
        headers: mockHeaders(crypto.randomUUID()),
      })
  }
}
