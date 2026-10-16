import type { ApiTransport } from '@/shared/api'

let csrfToken: string | null = null

export function setCsrfToken(token: string | null) {
  csrfToken = token
}

export const apiTransport: ApiTransport = {
  getCsrfToken: () => csrfToken,
}
