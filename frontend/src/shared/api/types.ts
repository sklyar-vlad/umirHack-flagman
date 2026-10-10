export interface Meta {
  server_time?: string
  request_id?: string
  [key: string]: unknown
}

export interface ListResponse<T, TMeta extends Meta = Meta> {
  items: T[]
  total: number
  page: number
  page_size: number
  meta: TMeta
}

export interface FieldViolation {
  path: string
  code: string
  message: string
}

export interface ErrorDetails {
  fields?: FieldViolation[]
  [key: string]: unknown
}

/** JSON-ответ backend; ошибки транспорта могут не иметь такого тела. */
export interface ErrorResponse {
  error: {
    code: string
    message: string
    details: ErrorDetails
  }
  request_id: string
}

/** Единая безопасная ошибка, возвращаемая baseQuery в RTK Query. */
export interface ApiError {
  status: number | 'NETWORK_ERROR' | 'TIMEOUT_ERROR' | 'INVALID_RESPONSE'
  code: string
  message: string
  details: ErrorDetails
  request_id?: string
  retry_after?: string
  http_status?: number
}

export interface HealthcheckResponse {
  status: 'ok'
  service: 'umirHack-flagman'
}
