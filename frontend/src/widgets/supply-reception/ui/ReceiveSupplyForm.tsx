import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useActiveProductsQuery } from '@/entities/product'
import { useReceiveSupplyMutation, type SupplyDetail } from '@/entities/supply'
import { clearWorkDraft, saveWorkDraft, selectSession } from '@/entities/session'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError, type ReceptionDraft, type ReceptionValues } from '@/shared/api'
import { Button, EmptyState, ErrorState, FormField, Input, Select, Skeleton } from '@/shared/ui'
import { canRetryReception, emptyReception, emptyReceptionLine, fieldViolationsSchema, receptionField, receptionPayload, receptionSchema, uncertainReception } from '@/features/receive-supply'
import styles from './ReceiveSupplyForm.module.css'

const unitLabels = { g: 'г', kg: 'кг', ml: 'мл', l: 'л', pcs: 'шт.' }
export function ReceiveSupplyForm({ onReceived }: { onReceived: (supply: SupplyDetail) => void }) {
  const session = useSelector(selectSession), dispatch = useDispatch()
  const [initial] = useState(() => session.draftOwner === session.user?.id ? session.drafts.reception?.reception : undefined)
  const owner = useRef(session.user?.id)
  const attempt = useRef(initial?.attempt)
  const busy = useRef(false)
  const mounted = useRef(true)
  const [unresolved, setUnresolved] = useState(!!initial?.attempt)
  const [notice, setNotice] = useState<string | null>(initial?.attempt ? 'Результат предыдущей отправки неизвестен. Повторите исходный запрос, чтобы выяснить результат.' : null)
  const [receive, mutation] = useReceiveSupplyMutation()
  const catalogue = useActiveProductsQuery(undefined, { refetchOnMountOrArgChange: true })
  const products = catalogue.currentData ?? []
  const timezone = session.settings?.timezone ?? 'Europe/Moscow'
  const form = useForm<ReceptionValues>({ defaultValues: initial?.values ?? emptyReception(), resolver: zodResolver(receptionSchema(products, timezone)) })
  const watchedItems = useWatch({ control: form.control, name: 'items' })
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'items' })
  const { errors, isDirty } = form.formState
  const locked = mutation.isLoading || unresolved
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => {
    return form.subscribe({ formState: { values: true }, callback: ({ values }) => {
      if (!values.items?.length) return
      const draft: ReceptionDraft = { values: form.getValues(), ...(attempt.current ? { attempt: attempt.current } : {}) }
      dispatch(saveWorkDraft({ key: 'reception', draft: { reception: draft } }))
    } })
  }, [dispatch, form])
  async function send(values?: ReceptionValues) {
    if (busy.current || session.user?.id !== owner.current) return
    if (attempt.current && !canRetryReception(attempt.current.startedAt)) {
      setNotice('Срок безопасного повтора истёк. Проверьте историю поставок и обратитесь к ответственному сотруднику для выяснения результата. Новая отправка заблокирована, чтобы избежать дубликата.')
      return
    }
    busy.current = true
    form.clearErrors()
    const current = attempt.current ?? { key: crypto.randomUUID(), body: receptionPayload(values!, products, timezone), startedAt: Date.now() }
    attempt.current = current
    dispatch(saveWorkDraft({ key: 'reception', draft: { reception: { values: form.getValues(), attempt: current } } }))
    setNotice(null)
    try {
      const supply = await receive({ key: current.key, body: current.body }).unwrap()
      if (!mounted.current) return
      attempt.current = undefined
      dispatch(clearWorkDraft('reception'))
      form.reset(emptyReception())
      // reset emits watch notifications; confirmed success must clear that draft too.
      dispatch(clearWorkDraft('reception'))
      onReceived(supply)
    } catch (error) {
      if (!mounted.current) return
      if (!isApiError(error) || uncertainReception(error)) {
        setUnresolved(true)
        setNotice('Результат отправки неизвестен. Форма сохранена. Повторите исходный запрос с тем же ключом; не создавайте вторую поставку.')
      } else {
        attempt.current = undefined
        setUnresolved(false)
        dispatch(saveWorkDraft({ key: 'reception', draft: { reception: { values: form.getValues() } } }))
        setNotice(error.status === 409 ? 'Номер поставки уже занят. Измените номер и отправьте новую попытку.' : error.message)
        let first: ReturnType<typeof receptionField> = null
        const violations = fieldViolationsSchema.safeParse(error.details.fields)
        if (violations.success) for (const violation of violations.data) {
          const field = receptionField(violation.path, current.body.items.length)
          if (field) { form.setError(field, { type: 'server', message: violation.message }); first ??= field }
        }
        if (error.status === 409 && error.code !== 'IDEMPOTENCY_CONFLICT') { form.setError('number', { type: 'server', message: 'Этот номер уже принят.' }); first ??= 'number' }
        if (first) form.setFocus(first)
      }
    } finally { busy.current = false }
  }
  return <>
    <UnsavedChangesGuard dirty={isDirty || unresolved || mutation.isLoading} />
    {catalogue.isFetching && <Skeleton label="Загрузка продуктов" rows={2} />}
    <ApiAccessBoundary error={catalogue.error}>
      {catalogue.error && <ErrorState title="Не удалось загрузить продукты" description="Форма сохранена. Повторите загрузку каталога перед новой отправкой." onRetry={() => { void catalogue.refetch() }} retrying={catalogue.isFetching} />}
      {!catalogue.isFetching && !catalogue.error && !products.length && <EmptyState title="Нет активных продуктов" description="Для новой приёмки нужен хотя бы один активный продукт." />}
      <form className={styles.form} noValidate onSubmit={(event) => { void form.handleSubmit((values) => send(values))(event) }} aria-label="Приёмка поставки">
        <p className="text-muted">Время ресторана: {timezone}. Одна операция принимает все строки. Актуальный срок годности окончательно проверяет сервер.</p>
        <fieldset disabled={locked} className={styles.fields}>
          <div className={styles.header}>
            <FormField label="Номер поставки" required error={errors.number?.message}>{(props) => <Input {...props} {...form.register('number')} autoComplete="off" />}</FormField>
            <FormField label="Дата приёмки" required error={errors.receivedAt?.message}>{(props) => <Input {...props} {...form.register('receivedAt')} type="datetime-local" step="1" />}</FormField>
          </div>
          <h2>Состав поставки</h2>
          <p className="text-caption text-muted">От 1 до 100 строк. Один продукт можно принять несколькими партиями.</p>
          {fields.map((field, index) => {
            const product = products.find((item) => item.id === watchedItems[index]?.productId)
            const units = product?.unit === 'pcs' ? ['pcs'] as const : product?.unit === 'ml' ? ['ml', 'l'] as const : ['g', 'kg'] as const
            return <fieldset key={field.id} className={styles.line}>
              <legend>Строка {index + 1}</legend>
              <div className={styles.grid}>
                <FormField label={`Продукт, строка ${index + 1}`} required error={errors.items?.[index]?.productId?.message}>{(props) => <Select {...props} {...form.register(`items.${index}.productId`, { onChange: (event) => {
                  const selected = products.find((item) => item.id === event.target.value)
                  if (selected) form.setValue(`items.${index}.displayUnit`, selected.display_unit, { shouldDirty: true })
                } })} disabled={catalogue.isFetching || !!catalogue.error}><option value="">Выберите продукт</option>{products.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</Select>}</FormField>
                <FormField label={`Количество, строка ${index + 1}`} required error={errors.items?.[index]?.quantity?.message}>{(props) => <Input {...props} {...form.register(`items.${index}.quantity`)} inputMode="decimal" autoComplete="off" />}</FormField>
                <FormField label={`Единица, строка ${index + 1}`}>{(props) => <Select {...props} {...form.register(`items.${index}.displayUnit`)} value={watchedItems[index]?.displayUnit ?? 'kg'}>{units.map((unit) => <option key={unit} value={unit}>{unitLabels[unit]}</option>)}</Select>}</FormField>
                <FormField label={`Годен до, строка ${index + 1}`} required error={errors.items?.[index]?.expiresAt?.message}>{(props) => <Input {...props} {...form.register(`items.${index}.expiresAt`)} type="datetime-local" step="1" />}</FormField>
              </div>
              <Button variant="ghost" disabled={fields.length === 1} onClick={() => remove(index)} aria-label={`Удалить строку ${index + 1}`}>Удалить строку</Button>
            </fieldset>
          })}
          {errors.items?.root?.message && <p role="alert">{errors.items.root.message}</p>}
          <Button variant="secondary" disabled={fields.length >= 100} onClick={() => append(emptyReceptionLine())}>Добавить строку</Button>
        </fieldset>
        {notice && <p role="alert" className={styles.notice}>{notice}</p>}
        {unresolved ? <Button onClick={() => { void send() }} loading={mutation.isLoading} loadingLabel="Выясняем результат…">Повторить исходный запрос</Button> : <Button type="submit" loading={mutation.isLoading} loadingLabel="Принимаем поставку…" disabled={catalogue.isFetching || !!catalogue.error || !products.length}>Принять поставку</Button>}
      </form>
    </ApiAccessBoundary>
  </>
}
