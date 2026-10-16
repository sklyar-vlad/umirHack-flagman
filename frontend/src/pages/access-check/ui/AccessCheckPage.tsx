import { useState } from 'react'
import { ApiAccessBoundary } from '@/features/access'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { useHealthcheckQuery } from '@/shared/api'
import { Button, ErrorState, FormField, Input, Skeleton } from '@/shared/ui'

export function AccessCheckPage() {
  const { error, isLoading, refetch } = useHealthcheckQuery()
  const [note, setNote] = useState('')
  return <><UnsavedChangesGuard dirty={note.length > 0} /><ApiAccessBoundary error={error}>
    <h1>Проверка доступа и переходов</h1>
    {isLoading ? <Skeleton /> : error ? <ErrorState onRetry={() => { void refetch() }} /> : <p>Пробный ответ получен. Это инструмент разработки.</p>}
    <FormField label="Тестовый черновик">{(props) => <Input {...props} value={note} onChange={(event) => setNote(event.target.value)} />}</FormField>
    <Button variant="secondary" onClick={() => setNote('')}>Очистить черновик</Button>
  </ApiAccessBoundary></>
}
