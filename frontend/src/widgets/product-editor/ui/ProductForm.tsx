import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { Link } from 'react-router'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useCreateProductMutation, useUpdateProductMutation, type Product } from '@/entities/product'
import { useProductCategoriesQuery } from '@/entities/inventory'
import { clearWorkDraft, saveWorkDraft, selectSession } from '@/entities/session'
import { emptyProduct, productFields, productFormSchema, productPatch, productToValues } from '@/features/manage-product'
import { CategoryDialog } from '@/features/manage-category'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError } from '@/shared/api'
import type { ProductDraft, ProductValues } from '@/shared/api/product-work'
import { Button, Checkbox, EmptyState, ErrorState, FormField, Input, Select, Skeleton } from '@/shared/ui'
import styles from './ProductForm.module.css'

const units = { g: ['g', 'kg'], ml: ['ml', 'l'], pcs: ['pcs'] } as const
const labels = { g: 'г', kg: 'кг', ml: 'мл', l: 'л', pcs: 'шт.' }
const fieldLabels = { name: 'Название', category_id: 'Категория (ID)', unit: 'Базовая единица', display_unit: 'Отображаемая единица', standard_shelf_life_hours: 'Стандартный срок, ч', target_stock: 'Целевой запас в базовой единице', is_active: 'Активность' }
export function ProductForm({ product, returnTo, onReload }: { product?: Product; returnTo: string; onReload?: () => Promise<Product | undefined> }) {
  const session = useSelector(selectSession), dispatch = useDispatch(), resourceId = product?.id ?? 'new', key = `product:${resourceId}`
  const [restored] = useState(() => session.draftOwner === session.user?.id ? session.drafts[key]?.product : undefined)
  const [baseline, setBaseline] = useState<ProductDraft['baseline']>(() => product ? { fields: productFields(productToValues(product)), revision: product.revision } : null)
  const [uncertain, setUncertain] = useState(restored?.uncertain ?? false)
  const [review, setReview] = useState(!!restored?.review || !!product && !!restored?.baseline && restored.baseline.revision !== product.revision)
  const [reloading, setReloading] = useState(false), [reloadFailed, setReloadFailed] = useState(false), [saved, setSaved] = useState<Product>()
  const [categoryOpen, setCategoryOpen] = useState(false), [categoryDirty, setCategoryDirty] = useState(false), [notice, setNotice] = useState('')
  const initiator = useRef<HTMLButtonElement>(null), submitting = useRef(false), owner = useRef(session.user?.id), latestSession = useRef(session)
  const [create, created] = useCreateProductMutation(), [update, updated] = useUpdateProductMutation()
  const { register, handleSubmit, control, setValue, reset, setError, formState: { errors } } = useForm<ProductValues>({ resolver: zodResolver(productFormSchema), defaultValues: restored?.values ?? (product ? productToValues(product) : emptyProduct) })
  const values = useWatch({ control }) as ProductValues
  const choices = useProductCategoriesQuery(undefined, { refetchOnMountOrArgChange: true })
  const mutation = product ? updated : created, error = isApiError(mutation.error) ? mutation.error : undefined
  const busy = mutation.isLoading || reloading, conflict = error?.code === 'REVISION_CONFLICT', missing = error?.status === 404, denied = error?.status === 403
  const changed = baseline ? Object.keys(productFormSchema.safeParse(values).success ? productPatch(values, baseline) : { invalid: true, expected_revision: baseline.revision }).length > 1 : JSON.stringify(values) !== JSON.stringify(emptyProduct)
  const dirty = !saved && (changed || uncertain || review || busy || categoryDirty)
  useEffect(() => { latestSession.current = session }, [session])
  useEffect(() => {
    if (saved) return
    if (dirty) dispatch(saveWorkDraft({ key, draft: { product: { resourceId, values, baseline, uncertain, review } } }))
    else dispatch(clearWorkDraft(key))
  }, [dispatch, key, resourceId, values, baseline, uncertain, review, dirty, saved])
  const locked = busy || reloadFailed || review || uncertain || conflict || missing || denied || !!baseline && baseline.revision > 2147483646
  const reload = async () => {
    if (!onReload || busy) return
    setReloading(true); setReloadFailed(false)
    try {
      const fresh = await onReload()
      if (fresh) { setBaseline({ fields: productFields(productToValues(fresh)), revision: fresh.revision }); updated.reset(); setReview(true) }
      else setReloadFailed(true)
    } finally { setReloading(false) }
  }
  const submit = async (input: ProductValues) => {
      if (locked || categoryOpen || submitting.current || choices.error || choices.isFetching || !choices.currentData?.some((category) => category.id === input.category_id) || product && !changed) return
      submitting.current = true; setUncertain(true)
      // Persist before sending so reload/expiration cannot turn an unknown write into an automatic replay.
      dispatch(saveWorkDraft({ key, draft: { product: { resourceId, values: input, baseline, uncertain: true, review } } }))
      try {
        const result = product && baseline ? await update({ id: product.id, body: productPatch(input, baseline) }) : await create(productFields(input))
        if (result.data && latestSession.current.status === 'authenticated' && owner.current === latestSession.current.user?.id) { reset(productToValues(result.data)); setSaved(result.data); dispatch(clearWorkDraft(key)); return }
        const failure = isApiError(result.error) ? result.error : undefined
        const unknown = !failure || typeof failure.status !== 'number' || failure.status >= 500
        setUncertain(unknown)
        if (failure?.status === 422) {
          for (const field of failure.details.fields ?? []) {
            const path = field.path.replace(/^\/?(?:body[./])?/, '')
            if (Object.hasOwn(fieldLabels, path)) setError(path as keyof ProductValues, { type: 'server', message: field.message }, { shouldFocus: true })
          }
        }
      } finally { submitting.current = false }
    }
  if (saved) return <section className={styles.form} aria-label="Результат сохранения продукта"><h2>Продукт сохранён: {saved.name}</h2><p role="status">Ревизия: {saved.revision}. Каталог и зависимые данные обновлены.</p><p>Стандартный срок применяется к новым партиям. Существующие партии и их сроки не изменены.</p><Link className="ui-button ui-button--primary" to={returnTo}>Вернуться в каталог</Link></section>
  return <section className={styles.form} aria-label="Черновик продукта">
    <UnsavedChangesGuard dirty={dirty} pending={busy} onDiscard={() => dispatch(clearWorkDraft(key))} />
    {restored && <p role="status">Восстановлен черновик продукта для текущего пользователя.</p>}
    <form aria-label={product ? 'Редактирование продукта' : 'Создание продукта'} onSubmit={(event) => { void handleSubmit(submit)(event) }}>
      <p>Название от 1 до 200 символов; уникальность и запреты изменения проверяет сервер.{baseline && ` Ревизия: ${baseline.revision}.`}</p>
      <fieldset disabled={busy || denied || categoryOpen} className={styles.fields}>
        <FormField label="Название продукта" error={errors.name?.message}>{(props) => <Input {...props} {...register('name')} aria-required autoFocus />}</FormField>
        <ApiAccessBoundary error={choices.error}>
          {choices.isFetching && !choices.currentData && <Skeleton label="Загрузка вариантов категорий" />}
          {!choices.isFetching && !choices.error && choices.currentData?.length === 0 && <EmptyState title="Категорий пока нет" description="Создайте категорию, чтобы сохранить продукт." />}
          {choices.error && <ErrorState title="Не удалось загрузить варианты категорий" onRetry={() => { void choices.refetch() }} />}
          <FormField label="Категория продукта" error={errors.category_id?.message}>{(props) => <Select {...props} {...register('category_id')} aria-required value={values.category_id} disabled={(choices.isFetching && !choices.currentData) || !!choices.error}><option value="">Выберите категорию</option>{values.category_id && !choices.currentData?.some((category) => category.id === values.category_id) && <option value={values.category_id}>Выбранная категория · обновление справочника</option>}{choices.currentData?.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</Select>}</FormField>
          <Button ref={initiator} variant="secondary" onClick={() => setCategoryOpen(true)}>Создать категорию для продукта</Button>
        </ApiAccessBoundary>
        <div className={styles.grid}>
          <FormField label="Базовая единица" hint="Смена единицы не преобразует введённую цель. При партиях или исторической рецептуре сервер запретит смену.">{(props) => <Select {...props} {...register('unit')} onChange={(event) => { const unit = event.target.value as ProductValues['unit']; setValue('unit', unit, { shouldDirty: true }); setValue('display_unit', unit, { shouldDirty: true }) }}>{(['g', 'ml', 'pcs'] as const).map((unit) => <option key={unit} value={unit}>{labels[unit]} ({unit})</option>)}</Select>}</FormField>
          <FormField label="Единица отображения" error={errors.display_unit?.message}>{(props) => <Select {...props} {...register('display_unit')} value={values.display_unit}>{units[values.unit].map((unit) => <option key={unit} value={unit}>{labels[unit]} ({unit})</option>)}</Select>}</FormField>
          <FormField label="Стандартный срок хранения, часы" error={errors.standard_shelf_life_hours?.message} hint="Пусто — стандарт не задан. От 1 до 2147483647 часов. Сроки существующих партий не меняются.">{(props) => <Input {...props} {...register('standard_shelf_life_hours')} inputMode="numeric" />}</FormField>
          <FormField label="Целевой запас" error={errors.target_stock?.message} hint={`В базовой единице: ${labels[values.unit]}. Пусто — цель не задана; 0 — нулевая цель. До 11 цифр и 3 знаков после разделителя; штуки целые.`}>{(props) => <Input {...props} {...register('target_stock')} inputMode="decimal" />}</FormField>
        </div>
        <Checkbox {...register('is_active')}>Продукт активен</Checkbox>
      </fieldset>
      {error && <p role="alert">{denied ? 'Недостаточно прав для изменения продукта.' : missing ? 'Продукт или категория не найдены. Ввод сохранён; проверьте каталог.' : conflict ? 'Продукт изменён другим пользователем. Загрузите актуальную версию и проверьте все поля.' : error.code === 'UNIT_LOCKED' ? 'Базовую единицу нельзя изменить: продукт связан с партиями или исторической рецептурой. Ввод сохранён.' : error.code === 'PRODUCT_IN_USE' ? 'Продукт нельзя отключить: он используется в активных блюдах. Ввод сохранён.' : error.code === 'CONFLICT' ? 'Продукт с таким названием уже существует.' : error.status === 422 ? 'Сервер отклонил поля продукта. Исправьте отмеченные значения.' : uncertain ? 'Результат сохранения неизвестен. Изменение могло сохраниться; повторная отправка заблокирована.' : 'Не удалось сохранить продукт. Ввод сохранён; повторите попытку после проверки.'}</p>}
      {error?.code === 'PRODUCT_IN_USE' && Array.isArray(error.details.dishes) && <ul>{error.details.dishes.filter((dish): dish is { id: string; name: string } => !!dish && typeof dish === 'object' && typeof dish.id === 'string' && typeof dish.name === 'string').map((dish) => <li key={dish.id}>{dish.name}</li>)}</ul>}
      {uncertain && !error && <p role="status">{busy ? 'Сохранение продукта…' : 'Результат предыдущего сохранения неизвестен. Проверьте каталог перед новой отправкой.'}</p>}
      {reloadFailed && <p role="alert">Не удалось загрузить актуальный продукт. Ввод сохранён; предыдущая ревизия не разрешена для повторной отправки.</p>}
      {review && baseline && <div role="status"><h2>Актуальный продукт: {baseline.fields.name}. Ревизия: {baseline.revision}.</h2><p>Ваш ввод сохранён. Сравните все поля: повторный PATCH применит ваши значения к этой версии.</p><dl className={styles.snapshot}>{Object.entries(baseline.fields).map(([field, value]) => <div key={field}><dt>{fieldLabels[field as keyof typeof fieldLabels]}</dt><dd>{value === null ? 'Не задан' : typeof value === 'boolean' ? value ? 'Активен' : 'Отключён' : String(value)}</dd></div>)}</dl><Button variant="secondary" disabled={reloadFailed || busy} onClick={() => { setReview(false); setUncertain(false) }}>Подтвердить актуальную ревизию</Button><Button variant="secondary" disabled={reloadFailed || busy} onClick={() => { reset({ ...baseline.fields, standard_shelf_life_hours: baseline.fields.standard_shelf_life_hours?.toString() ?? '', target_stock: baseline.fields.target_stock ?? '' }); setReview(false); setUncertain(false) }}>Использовать актуальные поля</Button></div>}
      {baseline && baseline.revision > 2147483646 && <p role="alert">Ревизия достигла предела изменения, обратитесь к администратору системы.</p>}
      {notice && <p role="status">{notice}</p>}
      <div className="ui-actions"><Button type="submit" loading={mutation.isLoading} loadingLabel="Сохранение продукта…" disabled={locked || categoryOpen || choices.isFetching || !!choices.error || !choices.currentData?.length || !!product && !changed}>Сохранить продукт</Button>{product && <Button variant="secondary" disabled={busy || categoryOpen} onClick={() => { void reload() }}>Загрузить актуальный продукт</Button>}</div>
      {!product && uncertain && !busy && <p><Link to={returnTo} target="_blank" rel="noreferrer">Проверить каталог в новой вкладке</Link> <Button variant="secondary" onClick={() => { setUncertain(false); created.reset() }}>Я проверил каталог — разрешить новую отправку</Button></p>}
    </form>
    {categoryOpen && <CategoryDialog returnFocusRef={initiator} onDirty={setCategoryDirty} onClose={() => { setCategoryOpen(false); setCategoryDirty(false) }} onSaved={(category) => { setValue('category_id', category.id, { shouldDirty: true }); setCategoryOpen(false); setCategoryDirty(false); setNotice(`Выбрана категория: ${category.name}. Черновик продукта сохранён.`) }} />}
  </section>
}
