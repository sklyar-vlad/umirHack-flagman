import {
  fetchBaseQuery,
  type BaseQueryFn,
  type FetchArgs,
  type FetchBaseQueryMeta,
} from '@reduxjs/toolkit/query'
import { normalizeApiError } from './api-error'
import type { ApiTransport } from './transport'
import type { ApiError } from './types'

const fetchQuery = fetchBaseQuery({
  baseUrl: '/api/v1',
  credentials: 'include',
  timeout: 15_000,
  prepareHeaders: (headers, { arg, extra }) => {
    headers.set('Accept', 'application/json')
    const method = typeof arg === 'string' ? 'GET' : (arg.method ?? 'GET').toUpperCase()
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
      const transport = extra as ApiTransport | undefined
      const token = transport?.getCsrfToken()
      if (token) headers.set('X-CSRF-Token', token)
      else headers.delete('X-CSRF-Token')
    }
    return headers
  },
})

// Один вызов fetch на попытку: мутации автоматически не повторяются.
export const baseQuery: BaseQueryFn<string | FetchArgs, unknown, ApiError, object, FetchBaseQueryMeta> = async (
  args,
  api,
  extraOptions,
) => {
  const transport = api.extra as ApiTransport | undefined
  const path = typeof args === 'string' ? args : args.url
  const protectedRequest = path !== '/healthcheck'
  if (protectedRequest && transport?.waitForSession && !await transport.waitForSession()) {
    return { error: { status: 401, code: 'SESSION_REQUIRED', message: 'Войдите перед продолжением работы.', details: {} } }
  }
  if (protectedRequest && transport?.hasSessionAccess && !transport.hasSessionAccess()) {
    return { error: { status: 401, code: 'SESSION_CHANGED', message: 'Сессия изменилась.', details: {} } }
  }
  if (api.signal.aborted) return { error: { status: 'NETWORK_ERROR', code: 'ABORTED', message: 'Запрос отменён.', details: {} } }
  const version = transport?.getSessionVersion?.()
  const result = await fetchQuery(args, api, extraOptions)
  if (protectedRequest && version !== transport?.getSessionVersion?.()) {
    return { error: { status: 401, code: 'SESSION_CHANGED', message: 'Сессия изменилась.', details: {} } }
  }
  if (protectedRequest && result.error && result.meta?.response?.status === 401) transport?.onUnauthorized?.()
  if (result.error) return { error: normalizeApiError(result.error, result.meta), meta: result.meta }
  return { data: result.data, meta: result.meta }
}
