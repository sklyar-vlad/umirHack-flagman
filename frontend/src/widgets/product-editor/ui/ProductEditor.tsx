import { useEffect, useState } from 'react'
import { useLazyProductQuery, type Product } from '@/entities/product'
import { isApiError, type ApiError } from '@/shared/api'
import { EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { ProductForm } from './ProductForm'

export function ProductEditor({ id, returnTo }: { id?: string; returnTo: string }) {
  const [read] = useLazyProductQuery({ refetchOnFocus: false, refetchOnReconnect: false })
  const [initial, setInitial] = useState<{ data?: Product; error?: ApiError }>({}), [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!id) return
    let active = true
    const request = read(id, false)
    void request.then((result) => {
      if (active) setInitial(result.error ? { error: isApiError(result.error) ? result.error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка чтения', details: {} } } : { data: result.data })
    })
    return () => { active = false; request.unsubscribe() }
  }, [id, attempt, read])
  if (!id) return <ProductForm returnTo={returnTo} />
  if (initial.error) return initial.error.status === 404 ? <EmptyState title="Продукт не найден" description="Сохранённый черновик не удалён. Проверьте адрес и каталог." /> : <ErrorState title={initial.error.status === 403 ? 'Недостаточно прав' : 'Не удалось прочитать продукт'} onRetry={() => { setInitial({}); setAttempt((value) => value + 1) }} />
  if (!initial.data) return <Skeleton label="Загрузка продукта для изменения" />
  return <ProductForm product={initial.data} returnTo={returnTo} onReload={async () => {
    const result = await read(id, false)
    return result.error ? undefined : result.data
  }} />
}
