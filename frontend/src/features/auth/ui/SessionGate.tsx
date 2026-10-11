import { useEffect } from 'react'
import { useSelector } from 'react-redux'
import { Navigate, Outlet, useLocation } from 'react-router'
import type { SessionState } from '@/entities/session'
import { ErrorState, Skeleton } from '@/shared/ui'
import { useAuth } from '../model/auth-context'
import { loginUrl } from '../model/return-url'

export function SessionGate() {
  const auth = useAuth()
  const session = useSelector((state: { session: SessionState }) => state.session)
  const location = useLocation()
  useEffect(() => { if (session.status === 'unknown') void auth.restore() }, [auth, session.status])
  useEffect(() => {
    if (session.status !== 'authenticated' || !session.expiresAt) return
    let timer: ReturnType<typeof setTimeout>
    function check() {
      const remaining = Date.parse(session.expiresAt!) - Date.now()
      if (remaining <= 0) auth.expire()
      else timer = setTimeout(check, Math.min(remaining, 2_147_483_647))
    }
    check()
    return () => clearTimeout(timer)
  }, [auth, session.status, session.expiresAt])
  if (session.status === 'unknown' || session.status === 'restoring' || session.status === 'signing-out') return <main className="app-placeholder ui-panel"><Skeleton label={session.status === 'signing-out' ? 'Выполняем выход' : 'Проверяем сессию'} /></main>
  if (session.status === 'error') return <main className="app-placeholder ui-panel"><ErrorState description={session.notice ?? undefined} onRetry={() => { void auth.restore() }} /></main>
  if (session.status !== 'authenticated') return <Navigate to={loginUrl(location.pathname === '/logout' ? '/' : location.pathname + location.search)} replace />
  return <Outlet />
}
