import { useEffect, useRef, useState } from 'react'
import { useDispatch } from 'react-redux'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { refreshSettingsViews, useUpdateSettingsMutation, type RestaurantSettings } from '@/entities/restaurant-settings'
import { acceptSettings } from '../model/accept-settings'
import { settingsField, settingsFormSchema, settingsPatch, settingsValues, type SettingsValues } from '@/features/manage-settings'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { isApiError, type ApiError } from '@/shared/api'
import { Button, Checkbox, Dialog, FormField, Input } from '@/shared/ui'
import styles from './SettingsEditor.module.css'

export function SettingsEditor({ initial, onReload, autoWriteoffReady = import.meta.env.VITE_AUTO_WRITEOFF_READY === 'true' }: {
  initial: RestaurantSettings; onReload: () => Promise<RestaurantSettings | undefined>; autoWriteoffReady?: boolean
}) {
  const dispatch = useDispatch(), [write, mutation] = useUpdateSettingsMutation()
  const [snapshot, setSnapshot] = useState(initial), [error, setError] = useState<ApiError>(), [success, setSuccess] = useState(false)
  const [review, setReview] = useState(false), [reloading, setReloading] = useState(false), [reloadFailed, setReloadFailed] = useState(false), [confirm, setConfirm] = useState<SettingsValues>()
  const [focusField, setFocusField] = useState<keyof SettingsValues>()
  const busy = useRef(false), mounted = useRef(true)
  const form = useForm<SettingsValues>({ resolver: zodResolver(settingsFormSchema), defaultValues: settingsValues(initial) })
  const { errors, isDirty } = form.formState
  const conflict = error?.code === 'REVISION_CONFLICT', denied = error?.status === 403
  const uncertain = error && (typeof error.status !== 'number' || error.status >= 500) && error.code !== 'FEATURE_UNAVAILABLE'
  const unavailable = error?.code === 'FEATURE_UNAVAILABLE'
  const locked = mutation.isLoading || reloading || !!conflict || review || !!denied || !!uncertain || snapshot.revision > 2147483646
  const auto = useWatch({ control: form.control, name: 'auto_writeoff_expired' })
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => {
    // A native disabled fieldset cannot receive focus until the pending render ends.
    if (focusField && !mutation.isLoading) form.setFocus(focusField)
  }, [focusField, mutation.isLoading, form])
  async function send(values: SettingsValues) {
    if (busy.current || locked) return
    const body = settingsPatch(values, snapshot)
    if (!body || body.auto_writeoff_expired === true && (!autoWriteoffReady || unavailable)) return
    busy.current = true; setFocusField(undefined); setConfirm(undefined); setSuccess(false); form.clearErrors()
    try {
      const fresh = await write(body).unwrap()
      if (!mounted.current) return
      setSnapshot(fresh); dispatch(acceptSettings(fresh)); form.reset(settingsValues(fresh)); setError(undefined); setSuccess(true)
    } catch (reason) {
      if (!mounted.current) return
      const failure: ApiError = isApiError(reason) ? reason : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка сохранения', details: {} }
      setError(failure)
      if (failure.status === 422) {
        // Only allowlisted field paths; never render arbitrary server details.
        const fields = Array.isArray(failure.details.fields) ? failure.details.fields : []
        let first: keyof SettingsValues | null = null
        for (const violation of fields) {
          if (!violation || typeof violation !== 'object' || !('path' in violation) || typeof violation.path !== 'string') continue
          const field = settingsField(violation.path)
          if (field) { form.setError(field, { type: 'server', message: 'Сервер отклонил это значение. Проверьте настройку.' }); first ??= field }
        }
        if (first) setFocusField(first)
      }
    } finally { busy.current = false }
  }
  async function reload() {
    if (busy.current) return
    busy.current = true; setReloading(true); setReloadFailed(false)
    try {
      const fresh = await onReload()
      if (!mounted.current) return
      if (fresh) { setSnapshot(fresh); dispatch(acceptSettings(fresh)); dispatch(refreshSettingsViews()); setError(undefined); setReview(true); setSuccess(false) } else setReloadFailed(true)
    } finally { busy.current = false; if (mounted.current) setReloading(false) }
  }
  return <>
    <UnsavedChangesGuard dirty={isDirty || mutation.isLoading || review} />
    <form aria-label="Настройки ресторана" className={styles.form} noValidate onSubmit={(event) => { void form.handleSubmit((values) => {
      if (busy.current || locked) return
      const body = settingsPatch(values, snapshot)
      if (body?.auto_writeoff_expired === true) { if (autoWriteoffReady && !unavailable) setConfirm(values) }
      else void send(values)
    })(event) }}>
      <p className="text-muted">Ревизия: {snapshot.revision}. Настройки действуют для всего ресторана.</p>
      <fieldset disabled={mutation.isLoading || reloading || denied} className={styles.fields}>
        <FormField label="Название ресторана" required error={errors.restaurant_name?.message}>{(props) => <Input {...props} {...form.register('restaurant_name')} autoComplete="off" />}</FormField>
        <FormField label="Часовой пояс IANA" required error={errors.timezone?.message} hint="Например, Europe/Moscow, Asia/Yekaterinburg или UTC.">{(props) => <Input {...props} {...form.register('timezone')} list="restaurant-timezones" autoComplete="off" />}</FormField>
        <datalist id="restaurant-timezones">{['Europe/Moscow', 'Asia/Yekaterinburg', 'Asia/Novosibirsk', 'Asia/Vladivostok', 'UTC'].map((zone) => <option key={zone} value={zone} />)}</datalist>
        <p className="text-caption text-muted">Часовой пояс меняет отображение времени, сохраняя исходные UTC-моменты. Даты в других черновиках потребуют пересмотра.</p>
        <FormField label="Порог предупреждения, часы" required error={errors.expiring_threshold_hours?.message} hint="Целое число от 1 до 720.">{(props) => <Input {...props} {...form.register('expiring_threshold_hours')} inputMode="numeric" />}</FormField>
        <p className="text-caption text-muted">Порог определяет, какие партии скоро истекут, и влияет на сводку и рекомендации. Статусы и остатки рассчитывает сервер.</p>
        <Checkbox {...form.register('auto_writeoff_expired')} checked={auto} disabled={!auto && (!autoWriteoffReady || unavailable)}>Автоматически списывать просроченные партии</Checkbox>
        {(!autoWriteoffReady || unavailable) && <p className="text-caption">Включение недоступно: готовность фонового списания не подтверждена. Уже включённое списание можно отключить.</p>}
        <p className="text-caption text-muted">При включении положительный просроченный остаток будет списываться в течение минуты. Отключение оставляет просрочку видимой и не возвращает уже списанный остаток.</p>
      </fieldset>
      {error && <p role="alert">{conflict ? 'Настройки изменены другим администратором. Прочитайте актуальную ревизию и проверьте ваши значения.' : unavailable ? 'Фоновое списание пока недоступно (FEATURE_UNAVAILABLE). Отключите переключатель и сохраните остальные настройки.' : denied ? 'Недостаточно прав для сохранения настроек.' : uncertain ? 'Результат сохранения неизвестен. Прочитайте настройки перед новой отправкой; автоматического повтора нет.' : error.status === 422 ? 'Проверьте поля формы. Изменения не сохранены.' : 'Не удалось сохранить настройки. Значения сохранены в форме.'}</p>}
      {reloadFailed && <p role="alert">Не удалось прочитать актуальные настройки. Ваши значения сохранены.</p>}
      {review && <section aria-label="Актуальные настройки" className={styles.review}><h2>Проверьте ревизию {snapshot.revision}</h2><p>Название: {snapshot.restaurant_name}. Зона: {snapshot.timezone}. Порог: {snapshot.expiring_threshold_hours} ч. Автосписание: {snapshot.auto_writeoff_expired ? 'включено' : 'выключено'}.</p><p>Ваши значения сохранены в форме. Сравните их перед повторной отправкой.</p><Button variant="secondary" onClick={() => { setReview(false); form.clearErrors() }}>Продолжить с этой ревизией</Button></section>}
      {snapshot.revision > 2147483646 && <p role="alert">Достигнут предел ревизии. Изменение недоступно.</p>}
      {success && <p role="status">Настройки сохранены. Время и зависимые сводки обновлены.</p>}
      <div className="ui-actions"><Button type="submit" loading={mutation.isLoading} loadingLabel="Сохраняем настройки…" disabled={locked || !isDirty || auto && !snapshot.auto_writeoff_expired && (!autoWriteoffReady || unavailable)}>Сохранить настройки</Button><Button variant="secondary" disabled={mutation.isLoading || reloading} onClick={() => { void reload() }}>Прочитать актуальные настройки</Button></div>
    </form>
    <Dialog open={!!confirm} title="Включить автоматическое списание?" onClose={() => setConfirm(undefined)} actions={<><Button variant="secondary" onClick={() => setConfirm(undefined)}>Отмена</Button><Button variant="danger" onClick={() => { if (confirm) void send(confirm) }}>Включить и сохранить</Button></>}><p>Просроченный положительный остаток будет списываться в течение минуты. Отключение функции не отменит проведённые списания.</p></Dialog>
  </>
}
