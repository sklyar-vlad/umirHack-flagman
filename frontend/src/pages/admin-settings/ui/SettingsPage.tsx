import { useEffect, useState } from 'react'
import { useDispatch } from 'react-redux'
import { refreshSettingsViews, useLazyRestaurantSettingsQuery, type RestaurantSettings } from '@/entities/restaurant-settings'
import { SettingsEditor, acceptSettings } from '@/widgets/settings-editor'
import { isApiError, type ApiError } from '@/shared/api'
import { ErrorState, Skeleton } from '@/shared/ui'

export function SettingsPage() {
  const dispatch = useDispatch(), [read] = useLazyRestaurantSettingsQuery({ refetchOnFocus: false, refetchOnReconnect: false })
  const [initial, setInitial] = useState<{ data?: RestaurantSettings; error?: ApiError }>({}), [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    const request = read(undefined, false)
    void request.then((result) => {
      if (!active) return
      if (result.data && !result.error) { setInitial({ data: result.data }); dispatch(acceptSettings(result.data)); dispatch(refreshSettingsViews()) }
      else setInitial({ error: isApiError(result.error) ? result.error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка чтения', details: {} } })
    })
    return () => { active = false; request.unsubscribe() }
  }, [read, attempt, dispatch])
  return <section><h1>Настройки ресторана</h1><p className="text-muted">Название, часовой пояс, предупреждения о сроках и автоматическое списание.</p>
    {initial.error ? <ErrorState title={initial.error.status === 403 ? 'Недостаточно прав' : 'Не удалось прочитать настройки'} onRetry={() => { setInitial({}); setAttempt((value) => value + 1) }} /> : initial.data ? <SettingsEditor initial={initial.data} onReload={async () => { const result = await read(undefined, false); return result.error ? undefined : result.data }} /> : <Skeleton label="Загрузка настроек ресторана" />}
  </section>
}
