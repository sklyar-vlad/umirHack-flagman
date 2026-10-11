import { useEffect, useRef } from 'react'
import { Skeleton } from '@/shared/ui'
import { useAuth } from '../model/auth-context'

/** Navigating here lets a dirty form block logout before any session data is cleared. */
export function SessionLogout() {
  const auth = useAuth()
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    void auth.logout()
  }, [auth])
  return <Skeleton label="Выполняем выход" />
}
