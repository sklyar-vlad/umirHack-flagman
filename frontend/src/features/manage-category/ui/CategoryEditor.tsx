import { useEffect, useState, type Ref } from 'react'
import { useLazyCategoryQuery, type Category } from '@/entities/product-category'
import { isApiError, type ApiError } from '@/shared/api'
import { EmptyState, ErrorState, Skeleton } from '@/shared/ui'
import { CategoryForm, type CategoryFormHandle } from './CategoryForm'

export function CategoryEditor({ id, onSaved, onCancel, onDirty, ref, focusRef }: {
  id: string
  focusRef?: { current: HTMLInputElement | null }
  onSaved: (category: Category) => void
  onCancel: () => void
  onDirty: (dirty: boolean) => void
  ref?: Ref<CategoryFormHandle>
}) {
  const [read] = useLazyCategoryQuery({ refetchOnFocus: false, refetchOnReconnect: false })
  const [initial, setInitial] = useState<{ data?: Category; error?: ApiError }>({})
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    const request = read(id, false)
    void request.then((result) => {
      if (active) setInitial(result.error ? { error: isApiError(result.error) ? result.error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка чтения', details: {} } } : { data: result.data })
    })
    return () => { active = false; request.unsubscribe() }
  }, [id, attempt, read])
  if (initial.error) return initial.error.status === 404 ? <EmptyState title="Категория не найдена" /> : <ErrorState title={initial.error.status === 403 ? 'Недостаточно прав' : 'Не удалось прочитать категорию'} onRetry={() => { setInitial({}); setAttempt((value) => value + 1) }} />
  if (!initial.data) return <Skeleton label="Загрузка категории для изменения" />
  return <CategoryForm ref={ref} focusRef={focusRef} category={initial.data} onSaved={onSaved} onCancel={onCancel} onDirty={onDirty} onReload={async () => {
    const result = await read(id, false)
    return result.error ? undefined : result.data
  }} />
}
