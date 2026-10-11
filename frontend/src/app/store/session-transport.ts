import type { ApiTransport } from '@/shared/api'

let csrfToken: string | null = null

/** CSRF lives in memory, outside Redux and browser storage. */
export function setCsrfToken(token: string | null) {
  csrfToken = token
}

export const apiTransport: ApiTransport = {
  getCsrfToken: () => csrfToken,
}
