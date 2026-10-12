import { useEffect, useRef, useState } from 'react'
import { useDispatch } from 'react-redux'
import type { ThunkDispatch, UnknownAction } from '@reduxjs/toolkit'
import { writeUser, type User, type UserWrite } from '@/entities/user'
import { useAuth } from '@/features/auth'
import { isApiError, type ApiError, type ApiTransport } from '@/shared/api'
export function useUserWrite() {
  const dispatch = useDispatch<ThunkDispatch<unknown, ApiTransport, UnknownAction>>(), auth = useAuth()
  const active = useRef<AbortController | null>(null), mounted = useRef(true)
  const [busy, setBusy] = useState(false), [error, setError] = useState<ApiError>()
  useEffect(() => {
    mounted.current = true
    const unregister = auth.registerSubscription(() => active.current?.abort())
    return () => { mounted.current = false; active.current?.abort(); unregister() }
  }, [auth])
  async function send(input: UserWrite): Promise<User | undefined> {
    if (active.current) return
    const controller = new AbortController(), version = auth.getVersion()
    active.current = controller; setBusy(true); setError(undefined)
    try {
      const user = await dispatch(writeUser(input, controller.signal))
      if (mounted.current && !controller.signal.aborted && version === auth.getVersion()) return user
    } catch (failure) {
      if (mounted.current && version === auth.getVersion()) setError(isApiError(failure) ? failure : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Не удалось сохранить пользователя.', details: {} })
    } finally { active.current = null; if (mounted.current) setBusy(false) }
  }
  return { send, busy, error, clearError: () => setError(undefined) }
}
export function userWriteMessage(error: ApiError) {
  if (error.code === 'REVISION_CONFLICT') return 'Пользователь изменён другим администратором. Прочитайте актуальную ревизию и проверьте свои значения.'
  if (['LAST_ADMIN', 'LAST_ADMIN_PROTECTED'].includes(error.code)) return 'Нельзя отключить или понизить последнего активного администратора. Ваши значения сохранены в форме.'
  if (error.status === 409 && error.code === 'CONFLICT') return 'Этот email уже используется. Укажите другой адрес.'
  if (error.status === 422) return 'Проверьте поля формы. Изменения не сохранены.'
  if (error.status === 403) return 'Недостаточно прав. Изменения не отправляйте до восстановления доступа.'
  if (error.status === 404) return 'Пользователь больше не найден. Вернитесь в список.'
  if (error.status === 429) return 'Слишком много запросов. Подождите перед повторной отправкой.'
  return 'Результат сохранения неизвестен. Проверьте список пользователей перед новой попыткой; запрос не повторяется автоматически.'
}
export const unknownUserWrite = (error?: ApiError) => !!error && (typeof error.status !== 'number' || error.status >= 500)
