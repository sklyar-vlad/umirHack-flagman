import { createApi } from '@reduxjs/toolkit/query/react'
import { z } from 'zod'
import { baseQuery } from './base-query'
import type { HealthcheckResponse } from './types'

const healthcheckSchema = z.object({
  status: z.literal('ok'),
  service: z.literal('umirHack-flagman'),
})

export const baseApi = createApi({
  reducerPath: 'api',
  baseQuery,
  tagTypes: ['Healthcheck'],
  refetchOnFocus: false,
  refetchOnReconnect: false,
  endpoints: (build) => ({
    healthcheck: build.query<HealthcheckResponse, void>({
      providesTags: ['Healthcheck'],
      queryFn: async (_arg, _api, _options, query) => {
        const result = await query('/healthcheck')
        if (result.error) return { error: result.error, meta: result.meta }
        const parsed = healthcheckSchema.safeParse(result.data)
        if (!parsed.success) {
          return {
            error: {
              status: 'INVALID_RESPONSE',
              code: 'INVALID_RESPONSE',
              message: 'Ответ сервера не соответствует формату healthcheck.',
              details: {},
              request_id: result.meta?.response?.headers.get('X-Request-ID') ?? undefined,
              retry_after: result.meta?.response?.headers.get('Retry-After') ?? undefined,
              http_status: result.meta?.response?.status,
            },
            meta: result.meta,
          }
        }
        return { data: parsed.data, meta: result.meta }
      },
    }),
  }),
})

export const { useHealthcheckQuery } = baseApi
