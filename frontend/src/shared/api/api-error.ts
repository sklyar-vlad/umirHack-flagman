import type { FetchBaseQueryError, FetchBaseQueryMeta } from '@reduxjs/toolkit/query'
import type { ApiError, ErrorResponse } from './types'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isErrorResponse(value: unknown): value is ErrorResponse {
  return isRecord(value)
    && typeof value.request_id === 'string'
    && isRecord(value.error)
    && typeof value.error.code === 'string'
    && typeof value.error.message === 'string'
    && isRecord(value.error.details)
}

export function isApiError(value: unknown): value is ApiError {
  return isRecord(value)
    && (typeof value.status === 'number'
      || value.status === 'NETWORK_ERROR'
      || value.status === 'TIMEOUT_ERROR'
      || value.status === 'INVALID_RESPONSE')
    && typeof value.code === 'string'
    && typeof value.message === 'string'
    && isRecord(value.details)
}

function httpMessage(status: number): string {
  if (status >= 500) return 'Сервер временно недоступен. Повторите запрос позже.'
  switch (status) {
    case 401: return 'Сессия отсутствует или истекла. Войдите в приложение.'
    case 403: return 'Недостаточно прав для выполнения действия.'
    case 404: return 'Ресурс или API-маршрут не найден.'
    case 413: return 'Объём отправленных данных слишком большой.'
    case 429: return 'Слишком много запросов. Подождите перед повтором.'
    default: return 'Не удалось выполнить запрос. Повторите попытку.'
  }
}

export function normalizeApiError(
  error: FetchBaseQueryError,
  meta?: FetchBaseQueryMeta,
): ApiError {
  const response = meta?.response
  const context = {
    request_id: response?.headers.get('X-Request-ID') ?? undefined,
    // Сохраняется исходное значение: число секунд или HTTP-date.
    retry_after: response?.headers.get('Retry-After') ?? undefined,
  }

  if (error.status === 'FETCH_ERROR') {
    return {
      ...context,
      status: 'NETWORK_ERROR',
      code: 'NETWORK_ERROR',
      message: 'Нет связи с сервером. Проверьте подключение к сети и доступность backend.',
      details: {},
    }
  }

  if (error.status === 'TIMEOUT_ERROR') {
    return {
      ...context,
      status: 'TIMEOUT_ERROR',
      code: 'TIMEOUT_ERROR',
      message: 'Сервер не ответил вовремя. Повторите запрос позже.',
      details: {},
    }
  }

  const httpStatus = typeof error.status === 'number'
    ? error.status
    : error.status === 'PARSING_ERROR' ? error.originalStatus : undefined

  if (httpStatus !== undefined && httpStatus >= 400) {
    const body = isErrorResponse(error.data) ? error.data : undefined
    return {
      ...context,
      status: httpStatus,
      code: body?.error.code ?? 'HTTP_ERROR',
      message: httpStatus >= 500 ? httpMessage(httpStatus) : body?.error.message ?? httpMessage(httpStatus),
      details: body?.error.details ?? {},
      request_id: body?.request_id ?? context.request_id,
    }
  }

  // Не возвращаем HTML прокси, текст парсера или внутренние исключения в UI/cache.
  return {
    ...context,
    status: 'INVALID_RESPONSE',
    code: 'INVALID_RESPONSE',
    message: 'Сервер вернул некорректный ответ. Повторите запрос позже.',
    details: {},
    http_status: httpStatus,
  }
}
