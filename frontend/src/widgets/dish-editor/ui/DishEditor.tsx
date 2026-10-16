import { useEffect, useState } from 'react'
import { useLazyDishQuery, type Dish } from '@/entities/dish'
import { isApiError, type ApiError } from '@/shared/api'
import { EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { DishForm } from './DishForm'

export function DishEditor({ id, returnTo }: { id?: string; returnTo: string }) {
  const [read] = useLazyDishQuery({ refetchOnFocus: false, refetchOnReconnect: false })
  const [initial, setInitial] = useState<{ data?: Dish; error?: ApiError }>({}), [attempt, setAttempt] = useState(0)
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
  if (!id) return <DishForm returnTo={returnTo} />
  if (initial.error) return initial.error.status === 404 ? <EmptyState title="Блюдо не найдено" description="Черновик сохранён. Проверьте адрес и каталог." /> : <ErrorState title={initial.error.status === 403 ? 'Недостаточно прав' : 'Не удалось прочитать блюдо'} onRetry={() => { setInitial({}); setAttempt((value) => value + 1) }} />
  if (!initial.data) return <Skeleton label="Загрузка блюда для изменения" />
  return <DishForm dish={initial.data} returnTo={returnTo} onReload={async () => {
    const request = read(id, false), result = await request
    request.unsubscribe()
    return result.error ? undefined : result.data
  }} />
}
