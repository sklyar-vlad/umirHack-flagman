import type { ReactNode } from 'react'
import { useSelector } from 'react-redux'
import { canAccess, type Section, type SessionState } from '@/entities/session'
import { Skeleton } from '@/shared/ui'
import { AccessDenied } from './AccessDenied'

/** Never mount a forbidden screen, including its query hooks. Place below SessionGate. */
export function RoleGate({ section, children }: { section: Section; children: ReactNode }) {
  const session = useSelector((state: { session: SessionState }) => state.session)
  if (session.status !== 'authenticated' || !session.user) return <Skeleton label="Проверяем права" />
  return canAccess(session.user.role, section) ? children : <AccessDenied />
}
