import type { ThunkDispatch, UnknownAction } from '@reduxjs/toolkit'
import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector, useStore } from 'react-redux'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link } from 'react-router'
import { useWriteOffMutation, type StockMovement } from '@/entities/stock-movement'
import { inventoryApi, type Batch } from '@/entities/inventory'
import { clearWorkDraft, saveWorkDraft, selectSession } from '@/entities/session'
import { canRetryWriteOff, uncertainWriteOff, writeOffField, writeOffFormSchema, writeOffPayload, writeOffViolations } from '@/features/write-off-batch'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { isApiError, type ApiError } from '@/shared/api'
import type { WriteOffDraft, WriteOffRequest, WriteOffValues } from '@/shared/api/write-off-work'
import { formatMovementQuantity, formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, Dialog, FormField, Input, Textarea } from '@/shared/ui'
import styles from './WriteOffDialog.module.css'

export function WriteOffDialog({ initial, returnFocusRef, onClose }: { initial: WriteOffDraft; returnFocusRef: { current: HTMLElement | null }; onClose: () => void }) {
  const dispatch = useDispatch<ThunkDispatch<{ session: ReturnType<typeof selectSession> }, unknown, UnknownAction>>(), store = useStore<{ session: ReturnType<typeof selectSession> }>()
  const session = useSelector(selectSession), owner = useRef(session.user?.id)
  const [snapshot, setSnapshot] = useState(initial), [attempt, setAttempt] = useState(initial.attempt)
  const [error, setError] = useState<ApiError>(), [result, setResult] = useState<StockMovement>()
  const [confirm, setConfirm] = useState<WriteOffRequest>(), [discard, setDiscard] = useState(false), [reloading, setReloading] = useState(false), [reloadError, setReloadError] = useState('')
  const [focusField, setFocusField] = useState<keyof WriteOffValues>()
  const [write, mutation] = useWriteOffMutation()
  const busy = useRef(false), mounted = useRef(true), request = useRef<{ abort: () => void } | null>(null)
  const form = useForm<WriteOffValues>({ resolver: zodResolver(writeOffFormSchema(snapshot)), defaultValues: initial.values })
  const values = useWatch({ control: form.control }), { errors, isDirty } = form.formState
  const authenticatedOwner = () => { const current = store.getState().session; return current.status === 'authenticated' && current.user?.id === owner.current }
  const persist = (pending: WriteOffDraft['attempt']) => {
    if (authenticatedOwner()) dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: { ...snapshot, values: form.getValues(), attempt: pending } } }))
  }
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false; request.current?.abort() }
  }, [])
  useEffect(() => {
    const current = store.getState().session
    if (!result && current.status === 'authenticated' && current.user?.id === owner.current) dispatch(saveWorkDraft({ key: 'writeOff', draft: { writeOff: { ...snapshot, values: { quantity: values.quantity ?? '', reason: values.reason ?? '' }, attempt } } }))
  }, [values.quantity, values.reason, snapshot, attempt, result, dispatch, store])
  useEffect(() => { if (focusField && !mutation.isLoading) form.setFocus(focusField) }, [focusField, mutation.isLoading, form])
  const locked = !!attempt || mutation.isLoading || reloading || error?.code === 'INSUFFICIENT_BATCH_STOCK' || error?.status === 404
  async function send(body?: WriteOffRequest) {
    if (busy.current || !authenticatedOwner() || result) return
    if (attempt && !canRetryWriteOff(attempt.startedAt)) { setError({ status: 409, code: 'RETRY_EXPIRED', message: 'Срок безопасного повтора истёк. Проверьте историю движений; новый запрос автоматически не создаётся.', details: {} }); return }
    const pending = attempt ?? (body ? { key: crypto.randomUUID(), body, startedAt: Date.now() } : undefined)
    if (!pending) return
    busy.current = true; setConfirm(undefined); setFocusField(undefined); form.clearErrors(); setError(undefined)
    setAttempt(pending); persist(pending)
    const call = write({ ...pending, productId: snapshot.product.id, unit: snapshot.batch.unit })
    request.current = call
    try {
      const movement = await call.unwrap()
      if (!mounted.current || !authenticatedOwner()) return
      setResult(movement); setAttempt(undefined); dispatch(clearWorkDraft('writeOff'))
    } catch (reason) {
      if (!mounted.current || !authenticatedOwner()) return
      const failure: ApiError = isApiError(reason) ? reason : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Нет подтверждения списания.', details: {} }
      setError(failure)
      if (!uncertainWriteOff(failure)) { setAttempt(undefined); persist(undefined) }
      if (failure.status === 422) {
        const fields = writeOffViolations.safeParse(failure.details.fields)
        let first: keyof WriteOffValues | undefined
        if (fields.success) for (const violation of fields.data) {
          const field = writeOffField(violation.path)
          if (field) { form.setError(field, { type: 'server', message: 'Сервер отклонил это значение. Проверьте поле.' }); first ??= field }
        }
        setFocusField(first)
      }
    } finally { busy.current = false; request.current = null }
  }
  async function refreshBatch() {
    if (busy.current || attempt) return
    busy.current = true; setReloading(true); setReloadError('')
    try {
      let fresh: Batch | undefined
      for (let page = 1; ; page++) {
        const query = dispatch(inventoryApi.endpoints.productBatches.initiate({ productId: snapshot.product.id, status: 'all', sort: 'expires_at', page, page_size: 100 }, { subscribe: false, forceRefetch: true }))
        request.current = query
        const data = await query.unwrap()
        fresh = data.items.find((batch) => batch.id === snapshot.batch.id)
        if (fresh || page * 100 >= data.total) break
        if (!data.items.length) throw new Error('Неполная история партий.')
      }
      if (!mounted.current || !authenticatedOwner()) return
      if (!fresh) { setReloadError('Партия больше не найдена. Сохраните причину и проверьте склад.'); return }
      if (fresh.unit !== snapshot.product.unit) throw new Error('Несовместимая единица партии.')
      setSnapshot({ ...snapshot, batch: fresh }); setError(undefined); form.clearErrors()
    } catch { if (mounted.current) setReloadError('Не удалось перечитать остаток. Значения формы сохранены; повторите чтение.') }
    finally { busy.current = false; request.current = null; if (mounted.current) setReloading(false) }
  }
  function close() {
    if (busy.current) return
    if (!result && !attempt && isDirty) { setDiscard(true); return }
    if (!attempt) dispatch(clearWorkDraft('writeOff'))
    onClose()
  }
  const batch = snapshot.batch, product = snapshot.product, timezone = session.settings?.timezone
  return <>
    <UnsavedChangesGuard dirty={!result && (isDirty || !!attempt || mutation.isLoading)} pending={mutation.isLoading || reloading} onDiscard={() => { if (!attempt) dispatch(clearWorkDraft('writeOff')) }} />
    <Dialog open title={result ? 'Списание выполнено' : 'Списать из партии'} onClose={close} returnFocusRef={returnFocusRef} description={result ? 'Движение подтверждено сервером.' : 'Списание уменьшает физический остаток выбранной партии.'}>
      {result ? <div className={styles.form}>
        <p>{result.product_name} · {formatMovementQuantity(result.delta_quantity, result.unit)}</p>
        <p>Остаток после движения: {formatQuantity(result.remaining_quantity_after, result.unit)}. Это исторический снимок.</p>
        <p>Причина: {result.note}</p><p>Автор: {result.actor?.name} · {formatRestaurantDateTime(result.occurred_at, timezone)}</p>
        <p className={styles.identity}>ID движения: {result.id}</p>
        <Link to={`/stock-movements?${new URLSearchParams({ batch_id: result.batch_id, product_id: result.product_id, type: 'manual_writeoff' })}`}>Открыть движение в истории</Link>
        <Button onClick={close}>Готово</Button>
      </div> : <form className={styles.form} aria-label="Ручное списание партии" noValidate onSubmit={(event) => { void form.handleSubmit((input) => { if (!locked && !busy.current) setConfirm(writeOffPayload(input, snapshot)) })(event) }}>
        <strong>{product.name}</strong>
        <p>{batch.supply.number} · строка {batch.line_number}</p>
        <p className={styles.identity}>ID партии: {batch.id}</p>
        <p>Срок: {formatRestaurantDateTime(batch.expires_at, timezone)}{batch.status === 'expired' && ' · Просрочена'}</p>
        <p>Физический остаток: {formatQuantity(batch.remaining_quantity, batch.unit, product.display_unit)}</p>
        <p className="text-caption text-muted">Остаток может измениться из-за заказа или автоматического списания. Окончательную проверку выполняет сервер.</p>
        <fieldset disabled={locked} className={styles.fields}>
          <FormField label="Количество списания" required hint={`Единица: ${formatQuantity('1000.000', batch.unit, product.display_unit).split(' ').at(-1)}. ${batch.unit === 'pcs' ? 'Только целые штуки.' : 'Точное количество; запятая или точка.'}`} error={errors.quantity?.message}>{(props) => <Input {...props} {...form.register('quantity')} inputMode="decimal" autoComplete="off" />}</FormField>
          <FormField label="Причина списания" required hint="1–500 символов без управляющих символов." error={errors.reason?.message}>{(props) => <Textarea {...props} {...form.register('reason')} rows={3} />}</FormField>
        </fieldset>
        {error && <p role="alert">{error.code === 'INSUFFICIENT_BATCH_STOCK' ? 'Остаток партии изменился: сервер отклонил списание. Перечитайте остаток; количество и причина сохранены.' : error.status === 404 ? 'Партия не найдена. Значения формы сохранены.' : error.status === 403 ? 'Недостаточно прав. Исходный запрос сохранён; проверьте доступ.' : error.message}</p>}
        {attempt && <p role="status">Результат отправки неизвестен. Редактирование заблокировано; можно повторить только исходный запрос с тем же ключом. Закрытие сохранит его для продолжения на складе.</p>}
        {reloadError && <p role="alert">{reloadError}</p>}
        {!attempt && error?.code === 'INSUFFICIENT_BATCH_STOCK' && <Button variant="secondary" loading={reloading} onClick={() => { void refreshBatch() }}>Перечитать остаток</Button>}
        {attempt ? <Button loading={mutation.isLoading} onClick={() => { void send() }}>Повторить исходный запрос</Button> : <Button type="submit" variant="danger" disabled={locked}>Списать</Button>}
        <Button variant="secondary" disabled={mutation.isLoading || reloading} onClick={close}>Закрыть списание</Button>
      </form>}
    </Dialog>
    <Dialog open={!!confirm} title="Подтвердить списание?" onClose={() => setConfirm(undefined)} description="Действие нельзя отменить в этом диалоге." actions={<><Button variant="secondary" onClick={() => setConfirm(undefined)}>Вернуться к форме</Button><Button variant="danger" onClick={() => { void send(confirm) }}>Подтвердить списание</Button></>}>
      {confirm && <p>{product.name} · {formatQuantity(confirm.quantity, batch.unit, product.display_unit)} · {confirm.reason}</p>}
    </Dialog>
    <Dialog open={discard} title="Закрыть без сохранения?" onClose={() => setDiscard(false)} actions={<><Button variant="secondary" onClick={() => setDiscard(false)}>Остаться</Button><Button variant="danger" onClick={() => { dispatch(clearWorkDraft('writeOff')); onClose() }}>Отбросить черновик</Button></>}><p>Количество и причина будут удалены.</p></Dialog>
  </>
}
