import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { Link } from 'react-router'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useCreateDishMutation, useUpdateDishMutation, usePreviewDishMutation, type Dish, type DishPreview } from '@/entities/dish'
import { useActiveProductsQuery } from '@/entities/product'
import { clearWorkDraft, saveWorkDraft, selectSession } from '@/entities/session'
import { emptyDish, dishFormSchema, dishToValues, dishFields, dishRecipe, dishBaseline, dishPatch, dishFieldPath } from '@/features/manage-dish'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { isApiError, type ApiError } from '@/shared/api'
import type { DishDraft, DishValues } from '@/shared/api/dish-work'
import { formatQuantity } from '@/shared/lib/quantity'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, Checkbox, EmptyState, ErrorState, FormField, Input, Select, Skeleton } from '@/shared/ui'
import styles from './DishForm.module.css'

type PreviewState = { marker: string; data?: DishPreview; error?: ApiError; loading?: boolean }
export function DishForm({ dish, returnTo, onReload }: { dish?: Dish; returnTo: string; onReload?: () => Promise<Dish | undefined> }) {
  const session = useSelector(selectSession), dispatch = useDispatch(), resourceId = dish?.id ?? 'new', key = `dish:${resourceId}`
  const [restored] = useState(() => session.draftOwner === session.user?.id ? session.drafts[key]?.dish : undefined)
  const [currentDish, setCurrentDish] = useState(dish)
  const [baselineValues, setBaselineValues] = useState(dish ? dishToValues(dish) : emptyDish)
  const [recipeVersion, setRecipeVersion] = useState(dish?.recipe_version)
  const [baseline, setBaseline] = useState<DishDraft['baseline']>(() => dish ? dishBaseline(dish) : null)
  const [uncertain, setUncertain] = useState(restored?.uncertain ?? false)
  const [review, setReview] = useState(!!restored?.review || !!dish && !!restored?.baseline && restored.baseline.revision !== dish.revision)
  const [reloading, setReloading] = useState(false), [reloadFailed, setReloadFailed] = useState(false), [saved, setSaved] = useState<Dish>()
  const [preview, setPreview] = useState<PreviewState>(), [previewInvalid, setPreviewInvalid] = useState(false)
  const [create, created] = useCreateDishMutation(), [update, updated] = useUpdateDishMutation(), [previewRecipe] = usePreviewDishMutation()
  const { register, handleSubmit, control, reset, setError, setValue, setFocus, trigger, getValues, formState: { errors } } = useForm<DishValues>({ resolver: zodResolver(dishFormSchema), defaultValues: restored?.values ?? (dish ? dishToValues(dish) : emptyDish) })
  const { fields, append, remove } = useFieldArray({ control, name: 'recipe' })
  const values = useWatch({ control }) as DishValues, marker = JSON.stringify(values)
  const products = useActiveProductsQuery(undefined, { refetchOnMountOrArgChange: true })
  const mutation = dish ? updated : created, error = isApiError(mutation.error) ? mutation.error : undefined
  const busy = mutation.isLoading || reloading, conflict = error?.code === 'REVISION_CONFLICT', missing = error?.status === 404, denied = error?.status === 403
  const valid = dishFormSchema.safeParse(values).success
  const changed = baseline ? (valid ? Object.keys(dishPatch(values, baseline)).length > 1 : marker !== JSON.stringify(baselineValues)) : marker !== JSON.stringify(emptyDish)
  const dirty = !saved && (changed || uncertain || review || busy)
  const locked = busy || reloadFailed || review || uncertain || conflict || missing || denied || !!baseline && baseline.revision > 2147483646
  const submitting = useRef(false), owner = useRef(session.user?.id), latestSession = useRef(session), sequence = useRef(0)
  const pendingPreview = useRef<ReturnType<typeof previewRecipe> | undefined>(undefined)
  const currentPreview = preview?.marker === marker ? preview : undefined
  useEffect(() => { latestSession.current = session }, [session])
  useEffect(() => () => { sequence.current++; pendingPreview.current?.abort() }, [])
  useEffect(() => {
    if (saved) return
    if (dirty) dispatch(saveWorkDraft({ key, draft: { dish: { resourceId, values, baseline, uncertain, review } } }))
    else dispatch(clearWorkDraft(key))
  }, [dispatch, key, resourceId, values, baseline, uncertain, review, dirty, saved])
  useEffect(() => {
    if (busy || uncertain || error?.status !== 422) return
    const path = error.details.fields?.map((field) => dishFieldPath(field.path)).find((path) => path !== undefined)
    if (path) setFocus(path)
  }, [busy, uncertain, error, setFocus])
  function invalidatePreview() {
    sequence.current++; pendingPreview.current?.abort(); pendingPreview.current = undefined
    setPreview(undefined); setPreviewInvalid(true)
  }
  function checkProducts(input: DishValues) {
    if (products.isFetching || products.error || !products.currentData) return false
    let good = true
    input.recipe.forEach((line, index) => {
      const product = products.currentData?.find((item) => item.id.toLowerCase() === line.product_id.toLowerCase())
      if (!product || product.unit !== line.unit) { good = false; setError(`recipe.${index}.product_id`, { message: 'Выберите актуальный активный продукт' }, { shouldFocus: true }) }
    })
    return good
  }
  const runPreview = async (input: DishValues) => {
    if (busy || pendingPreview.current || !checkProducts(input)) return
    const requestMarker = JSON.stringify(input), serial = ++sequence.current
    setPreviewInvalid(false); setPreview({ marker: requestMarker, loading: true })
    const request = previewRecipe({ recipe: dishRecipe(input) }); pendingPreview.current = request
    try {
      const result = await request
      if (serial !== sequence.current) return
      setPreview(result.data ? { marker: requestMarker, data: result.data } : { marker: requestMarker, error: isApiError(result.error) ? result.error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Не удалось проверить состав', details: {} } })
      if (isApiError(result.error)) applyFields(result.error)
    } finally { if (serial === sequence.current) pendingPreview.current = undefined }
  }
  function applyFields(failure: ApiError) {
    for (const field of failure.details.fields ?? []) {
      const path = dishFieldPath(field.path)
      if (path) setError(path, { type: 'server', message: field.message }, { shouldFocus: true })
    }
  }
  const reload = async () => {
    if (!onReload || busy) return
    invalidatePreview(); setReloading(true); setReloadFailed(false)
    try {
      const fresh = await onReload()
      if (fresh) { setBaseline(dishBaseline(fresh)); setCurrentDish(fresh); setBaselineValues(dishToValues(fresh)); setRecipeVersion(fresh.recipe_version); updated.reset(); setReview(true) }
      else setReloadFailed(true)
    } finally { setReloading(false) }
  }
  const submit = async (input: DishValues) => {
    if (locked || submitting.current || dish && !changed) return
    const body = baseline ? dishPatch(input, baseline) : dishFields(input)
    if (('recipe' in body || body.is_active === true && baseline?.fields.is_active === false) && !checkProducts(input)) return
    submitting.current = true; invalidatePreview(); setUncertain(true)
    dispatch(saveWorkDraft({ key, draft: { dish: { resourceId, values: input, baseline, uncertain: true, review } } }))
    try {
      const result = dish && baseline ? await update({ id: dish.id, body: dishPatch(input, baseline), previousVersion: recipeVersion }) : await create(dishFields(input))
      if (result.data && latestSession.current.status === 'authenticated' && owner.current === latestSession.current.user?.id) { reset(dishToValues(result.data)); setSaved(result.data); dispatch(clearWorkDraft(key)); return }
      const failure = isApiError(result.error) ? result.error : undefined
      setUncertain(!failure || typeof failure.status !== 'number' || failure.status >= 500)
      if (failure?.status === 422) applyFields(failure)
    } finally { submitting.current = false }
  }
  if (saved) return <section className={styles.form} aria-label="Результат сохранения блюда"><h2>Блюдо сохранено: {saved.name}</h2><p role="status">Ревизия: {saved.revision}. Версия рецептуры: {saved.recipe_version ?? 'нет'}. Каталог и зависимые данные обновлены.</p><ul>{saved.recipe.map((line) => <li key={line.product_id}>{line.product_name} — {formatQuantity(line.quantity_per_portion, line.unit)} на порцию</li>)}</ul><p>Исторические рецептуры и заказы сохранены. Корзина проходит повторную проверку.</p><Link className="ui-button ui-button--primary" to={returnTo}>Вернуться в каталог</Link></section>
  return <section className={styles.form} aria-label="Черновик блюда">
    <UnsavedChangesGuard dirty={dirty} pending={busy} onDiscard={() => dispatch(clearWorkDraft(key))} />
    {restored && <p role="status">Восстановлен черновик блюда для текущего пользователя.</p>}
    <form aria-label={dish ? 'Редактирование рецептуры' : 'Создание рецептуры'} onChange={invalidatePreview} onSubmit={(event) => { void handleSubmit(submit)(event) }}>
      <p>Состав на одну порцию; от 1 до 100 ингредиентов. Расход в базовых единицах, до 11 цифр и 3 знаков после разделителя; штуки целые.{baseline && ` Ревизия: ${baseline.revision}.`}</p>
      <fieldset disabled={busy || denied || uncertain} className={styles.fields}>
        <FormField label="Название блюда" required error={errors.name?.message}>{(props) => <Input {...props} {...register('name')} autoFocus />}</FormField>
        <FormField label="Описание блюда" hint="До 2000 символов. Пусто — без описания." error={errors.description?.message}>{(props) => <textarea {...props} {...register('description')} className={styles.description} rows={3} />}</FormField>
        <Checkbox {...register('is_active')}>Блюдо активно</Checkbox>
        <h2>Рецептура на порцию</h2>
        {products.isFetching && !products.currentData && <Skeleton label="Загрузка активных продуктов" />}
        {products.error && <ErrorState title={isApiError(products.error) && products.error.status === 403 ? 'Недостаточно прав для чтения продуктов' : 'Не удалось загрузить активные продукты'} onRetry={() => { void products.refetch() }} />}
        {!products.isFetching && !products.error && products.currentData?.length === 0 && <EmptyState title="Активных продуктов пока нет" description="Добавьте продукт в административном каталоге." />}
        {(errors.recipe?.root?.message ?? errors.recipe?.message) && <p role="alert">{errors.recipe?.root?.message ?? errors.recipe?.message}</p>}
        {fields.map((field, index) => {
          const line = values.recipe[index], selected = products.currentData?.find((product) => product.id.toLowerCase() === line?.product_id.toLowerCase())
          return <fieldset key={field.id} className={styles.ingredient}><legend>Ингредиент {index + 1}</legend>
            <FormField label={`Продукт ${index + 1}`} required error={errors.recipe?.[index]?.product_id?.message}>{(props) => <Select {...props} {...register(`recipe.${index}.product_id`)} value={line?.product_id ?? ''} disabled={(products.isFetching && !products.currentData) || !!products.error} onChange={(event) => {
              invalidatePreview(); setValue(`recipe.${index}.product_id`, event.target.value, { shouldDirty: true, shouldValidate: true }); const product = products.currentData?.find((item) => item.id === event.target.value)
              if (product) setValue(`recipe.${index}.unit`, product.unit, { shouldDirty: true })
            }}><option value="">Выберите активный продукт</option>{line?.product_id && !selected && <option value={line.product_id}>{dish?.recipe.find((item) => item.product_id.toLowerCase() === line.product_id.toLowerCase())?.product_name ?? 'Сохранённый продукт'} · недоступен в активном каталоге</option>}{products.currentData?.map((product) => <option key={product.id} value={product.id} disabled={values.recipe.some((item, other) => other !== index && item.product_id.toLowerCase() === product.id.toLowerCase())}>{product.name} ({product.unit})</option>)}</Select>}</FormField>
            <FormField label={`Расход ${index + 1}`} required hint={`Базовая единица: ${selected?.unit ?? line?.unit ?? 'g'}`} error={errors.recipe?.[index]?.quantity_per_portion?.message}>{(props) => <Input {...props} {...register(`recipe.${index}.quantity_per_portion`)} inputMode="decimal" />}</FormField>
            <Button variant="ghost" disabled={fields.length <= 1} aria-label={`Удалить ингредиент ${index + 1}`} onClick={() => { invalidatePreview(); remove(index) }}>Удалить ингредиент</Button>
          </fieldset>
        })}
        <Button variant="secondary" disabled={fields.length >= 100} onClick={() => { invalidatePreview(); append({ product_id: '', quantity_per_portion: '', unit: 'g' }) }}>Добавить ингредиент</Button>
      </fieldset>
      {error && <p role="alert">{denied ? 'Недостаточно прав для изменения блюда.' : missing ? 'Блюдо или продукт не найдены. Ввод сохранён.' : conflict ? 'Блюдо изменено другим пользователем. Загрузите актуальную версию и проверьте все поля.' : error.code === 'DUPLICATE_INGREDIENT' ? 'Продукт повторяется в составе. Удалите дубликат; черновик сохранён.' : error.status === 422 ? 'Сервер отклонил поля рецептуры. Исправьте отмеченные значения; черновик сохранён.' : error.code === 'CONFLICT' ? 'Блюдо с таким названием уже существует.' : uncertain ? 'Результат сохранения неизвестен. Автоматический повтор заблокирован. Проверьте каталог и актуальную версию.' : error.message}</p>}
      {uncertain && <p role="alert">Черновик сохранён. Сверьте каталог перед новой записью, чтобы исключить дубликат.</p>}
      {uncertain && !dish && <><a href={returnTo} target="_blank" rel="noreferrer">Проверить каталог в новой вкладке</a><Button variant="secondary" disabled={busy} onClick={() => { setUncertain(false); created.reset() }}>Я проверил каталог — разрешить новую отправку</Button></>}
      {(conflict || uncertain || review || reloadFailed) && dish && <Button variant="secondary" loading={reloading} onClick={() => { void reload() }}>Загрузить актуальную версию блюда</Button>}
      {reloadFailed && <p role="alert">Не удалось загрузить актуальную версию. Черновик сохранён; повторите чтение.</p>}
      {review && baseline && <section className={styles.review} aria-label="Проверка актуальной версии"><h2>Проверьте актуальную версию</h2><p>Ревизия: {baseline.revision}. Ваши введённые поля сохранены.</p><p>Название: {baseline.fields.name}. Описание: {baseline.fields.description ?? 'нет'}. {baseline.fields.is_active ? 'Активно' : 'Отключено'}</p><ul>{currentDish?.recipe.map((line) => <li key={line.product_id}>{line.product_name} — {formatQuantity(line.quantity_per_portion, line.unit)} на порцию</li>)}</ul><Button disabled={busy || reloadFailed} onClick={() => { setReview(false); setUncertain(false) }}>Подтверждаю проверку всех полей блюда</Button></section>}
      <div className={styles.actions}><Button variant="secondary" disabled={busy || denied || products.isFetching || !!products.error || !!currentPreview?.loading} loading={!!currentPreview?.loading} loadingLabel="Проверяем рецептуру…" onClick={() => { void trigger('recipe').then((valid) => { if (valid) void runPreview(getValues()) }) }}>Проверить без сохранения</Button><Button type="submit" disabled={locked || !!dish && !changed} loading={mutation.isLoading} loadingLabel="Сохраняем блюдо…">{dish ? 'Сохранить изменения блюда' : 'Создать блюдо'}</Button></div>
      <p className="text-caption text-muted">Preview не резервирует продукты и не сохраняет блюдо. Сохранение нового состава создаёт целую новую версию; изменение только имени, описания или активности сохраняет версию.</p>
    </form>
    {previewInvalid && <p role="status">Предыдущая проверка неактуальна. Проверьте текущие поля снова.</p>}
    {currentPreview?.error && <ErrorState title="Не удалось проверить рецептуру" description={currentPreview.error.message} onRetry={() => { void trigger('recipe').then((valid) => { if (valid) void runPreview(getValues()) }) }} />}
    {currentPreview?.data && <section className={styles.preview} aria-label="Проверка рецептуры"><h2>Проверка рецептуры</h2><p role="status">{currentPreview.data.is_available ? 'Можно приготовить' : 'Есть дефицит'}. Доступные порции: {currentPreview.data.available_portions}.</p><p>Снимок сервера: {formatRestaurantDateTime(currentPreview.data.meta.server_time, session.settings?.timezone ?? 'Europe/Moscow')}</p><ul>{currentPreview.data.requirements.map((line) => <li key={line.product_id}><strong>{line.product_name}</strong>: требуется {formatQuantity(line.required_quantity, line.unit)}; доступно {formatQuantity(line.available_quantity, line.unit)}; не хватает {formatQuantity(line.missing_quantity, line.unit)}</li>)}</ul><ul>{currentPreview.data.shortages.map((line) => <li key={line.product_id}>{line.product_name}: {line.reason_code === 'only_expired_stock' ? 'Осталась только просрочка' : 'Недостаточно годного остатка'}; просрочено {formatQuantity(line.expired_quantity, line.unit)}</li>)}</ul></section>}
  </section>
}
