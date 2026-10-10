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
  const result = await fetchQuery(args, api, extraOptions)
  if (result.error) return { error: normalizeApiError(result.error, result.meta), meta: result.meta }
  return { data: result.data, meta: result.meta }
}
