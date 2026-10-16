import { useEffect, useState } from 'react'
import { useLazyUserQuery, type User } from '@/entities/user'
import { isApiError, type ApiError } from '@/shared/api'
import { EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { UserForm } from './UserForm'
export function UserEditor({ id, password = false }: { id?: string; password?: boolean }) {
  const [read] = useLazyUserQuery({ refetchOnFocus: false, refetchOnReconnect: false })
  const [initial, setInitial] = useState<{ data?: User; error?: ApiError }>({}), [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!id) return
    let active = true
    const request = read(id, false)
    void request.then((result) => {
      if (active) setInitial(result.error ? { error: isApiError(result.error) ? result.error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка чтения', details: {} } } : { data: result.data })
    })
    request.unsubscribe()
    return () => { active = false }
  }, [id, attempt, read])
  if (!id) return <UserForm />
  if (initial.error) return initial.error.status === 404 ? <EmptyState title="Пользователь не найден" description="Проверьте адрес и список пользователей." /> : <ErrorState title={initial.error.status === 403 ? 'Недостаточно прав' : 'Не удалось прочитать пользователя'} onRetry={() => { setInitial({}); setAttempt((value) => value + 1) }} />
  if (!initial.data) return <Skeleton label="Загрузка пользователя для изменения" />
  return <UserForm user={initial.data} password={password} onReload={async () => {
    const request = read(id, false), result = await request
    request.unsubscribe()
    if (result.error) throw result.error
    return result.data!
  }} />
}
