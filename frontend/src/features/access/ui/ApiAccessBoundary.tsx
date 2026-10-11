import type { ReactNode } from 'react'
import { isApiError } from '@/shared/api'
import { AccessDenied } from './AccessDenied'

/** Handle server permissions locally; a 403 must not expire an authenticated session. */
export function ApiAccessBoundary({ error, children }: { error: unknown; children: ReactNode }) {
  return isApiError(error) && error.status === 403 ? <AccessDenied /> : children
}
