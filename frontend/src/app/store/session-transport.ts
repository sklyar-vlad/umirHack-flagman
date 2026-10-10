import type { ApiTransport } from '@/shared/api'

let csrfToken: string | null = null

/** Сессия будет обновлять токен при входе/me и очищать при выходе (C11–C12). */
export function setCsrfToken(token: string | null) {
  csrfToken = token
}

export const apiTransport: ApiTransport = {
  getCsrfToken: () => csrfToken,
}
