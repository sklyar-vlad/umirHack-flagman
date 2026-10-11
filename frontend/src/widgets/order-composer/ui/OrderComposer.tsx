import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useLazyDishQuery, type Dish } from '@/entities/dish'
import { clearWorkDraft, replaceBasket, saveWorkDraft, selectSession } from '@/entities/session'
import { orderRefusalSchema, useCheckOrderMutation, useCreateOrderMutation, type OrderCheck, type OrderDetail, type OrderRefusal } from '@/entities/order'
import { addBasketDish, basketFingerprint, basketRequest, canRetryOrder, portionsSchema, recipeChanged, uncertainOrder } from '@/features/compose-order'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { isApiError, type ApiError } from '@/shared/api'
import { basketDishSchema, type OrderDraft } from '@/shared/api/order-work'
import { formatQuantity } from '@/shared/lib/quantity'
import { Button, EmptyState, ErrorState, FormField, Input } from '@/shared/ui'
import { DishCatalogue } from './DishCatalogue'
import { OrderRequirements } from './OrderRequirements'
import styles from './OrderComposer.module.css'

const emptyDraft: OrderDraft = { lines: [] }
type CheckState = { marker: string; data?: OrderCheck; error?: ApiError; loading?: boolean }
export function OrderComposer({ onCreated }: { onCreated: (order: OrderDetail) => void }) {
  const session = useSelector(selectSession), dispatch = useDispatch()
  const draft = session.draftOwner === session.user?.id ? session.drafts.order?.order ?? emptyDraft : emptyDraft
  const [check, setCheck] = useState<CheckState>()
  const [revision, setRevision] = useState(0)
  const [latest, setLatest] = useState<Record<string, Dish>>({})
  const [reviewRequired, setReviewRequired] = useState(false)
  const [reviewReady, setReviewReady] = useState(false)
  const [notice, setNotice] = useState<string | null>(draft.attempt ? 'Результат предыдущей отправки неизвестен. Повторите исходный запрос.' : null)
  const [refusal, setRefusal] = useState<OrderRefusal>()
  const [blocked, setBlocked] = useState(false)
  const [retryAt, setRetryAt] = useState(0)
  const [checkOrder] = useCheckOrderMutation(), [createOrder, mutation] = useCreateOrderMutation(), [loadDish, dishQuery] = useLazyDishQuery()
  const owner = useRef(session.user?.id)
  const busy = useRef(false), mounted = useRef(true)
  const attempt = useRef(draft.attempt)
  const fingerprint = basketFingerprint(draft), marker = `${revision}:${fingerprint}`
  const body = useMemo(() => basketRequest(draft), [draft])
  const changed = draft.lines.filter((line) => latest[line.dish.id] && recipeChanged(line.dish, latest[line.dish.id]))
  const needsReview = reviewRequired || changed.length > 0
  const locked = !!draft.attempt || mutation.isLoading
  const current = check?.marker === marker ? check : undefined
  const canCreate = !!body && !!current?.data?.can_create && !needsReview && !locked && !blocked
  const observed = useCallback((dish: Dish) => setLatest((prior) => prior[dish.id] === dish || prior[dish.id]?.revision > dish.revision ? prior : { ...prior, [dish.id]: dish }), [])
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  function save(next: OrderDraft) {
    dispatch(saveWorkDraft({ key: 'order', draft: { order: next } }))
  }
  function edit(next: OrderDraft) {
    if (locked || busy.current) return
    save(next); setRevision((value) => value + 1); setBlocked(false); setRefusal(undefined); setNotice(null)
  }
  async function refreshRecipes(ids: string[]) {
    const results = await Promise.allSettled(ids.map((id) => loadDish(id, false).unwrap()))
    if (!mounted.current) return
    for (const result of results) if (result.status === 'fulfilled') observed(result.value)
    setReviewReady(results.every((result) => result.status === 'fulfilled'))
    if (results.some((result) => result.status === 'rejected')) setNotice('Не удалось загрузить актуальный состав. Корзина сохранена; повторите загрузку рецептур.')
  }
  useEffect(() => {
    if (!body || locked || needsReview || blocked) return
    let active = true
    let request: ReturnType<typeof checkOrder> | undefined
    const timer = window.setTimeout(() => {
      setCheck({ marker, loading: true })
      request = checkOrder(body)
      void request.unwrap().then((data) => {
        if (!active) return
        setCheck({ marker, data })
        const ids = data.issues.filter((issue) => issue.code === 'recipe_changed' || issue.code === 'dish_inactive' || issue.code === 'recipe_missing').map((issue) => issue.dish_id)
        if (ids.length) {
          setReviewRequired(true); setReviewReady(false)
          setLatest((prior) => Object.fromEntries(Object.entries(prior).filter(([id]) => !ids.includes(id))))
          void Promise.allSettled(ids.map((id) => loadDish(id, false).unwrap())).then((results) => {
            if (!mounted.current) return
            for (const result of results) if (result.status === 'fulfilled') observed(result.value)
            setReviewReady(results.every((result) => result.status === 'fulfilled'))
            if (results.some((result) => result.status === 'rejected')) setNotice('Не удалось загрузить актуальный состав. Повторите загрузку рецептур.')
          })
        }
      }).catch((error: unknown) => { if (active && isApiError(error)) setCheck({ marker, error }) })
    }, 300)
    return () => { active = false; window.clearTimeout(timer); request?.abort() }
  }, [body, marker, locked, needsReview, blocked, checkOrder, loadDish, observed])
  async function send() {
    if (session.user?.id !== owner.current || busy.current || (!attempt.current && !canCreate)) return
    if (Date.now() < retryAt) { setNotice('Подождите перед повтором исходного запроса.'); return }
    if (attempt.current && !canRetryOrder(attempt.current.startedAt)) { setNotice('Срок безопасного повтора истёк. Проверьте историю заказов и обратитесь к ответственному сотруднику. Новая отправка заблокирована во избежание дубликата.'); return }
    const original = attempt.current ?? { key: crypto.randomUUID(), body: body!, startedAt: Date.now() }
    busy.current = true; attempt.current = original
    save({ ...draft, attempt: original }); setNotice(null)
    try {
      const order = await createOrder({ key: original.key, body: original.body }).unwrap()
      if (!mounted.current) return
      attempt.current = undefined
      dispatch(clearWorkDraft('order')); dispatch(replaceBasket([])); onCreated(order)
    } catch (error) {
      if (!mounted.current) return
      if (!isApiError(error) || uncertainOrder(error)) {
        if (isApiError(error) && error.status === 429) setRetryAt(Date.now() + (Number(error.retry_after) || 1) * 1000)
        setNotice('Результат отправки неизвестен. Корзина сохранена и заблокирована. Повторите исходный запрос с тем же ключом. Новый заказ не создавайте.' + (isApiError(error) && error.status === 429 ? ` Лимит запросов: подождите ${error.retry_after ?? '1'} с.` : ''))
      } else {
        attempt.current = undefined; save({ lines: draft.lines }); setBlocked(true); setRevision((value) => value + 1)
        const parsed = orderRefusalSchema.safeParse(error.details)
        setRefusal(parsed.success ? parsed.data : undefined)
        setNotice(error.message + (parsed.success ? ` Сохранённая попытка: ${parsed.data.attempt_id}.` : '') + ' Корзина сохранена.')
        if (error.code === 'RECIPE_CHANGED' || error.code === 'DISH_UNAVAILABLE') {
          setReviewRequired(true); setReviewReady(false)
          setLatest({})
          void refreshRecipes(draft.lines.map((line) => line.dish.id))
        }
        if (error.status === 422) {
          const fields = z.array(z.object({ path: z.string(), message: z.string() })).safeParse(error.details.fields)
          if (fields.success) setNotice(`${error.message} ${fields.data.map((field) => `${field.path}: ${field.message}`).join(' ')} Корзина сохранена.`)
        }
      }
    } finally { busy.current = false }
  }
  return <div className={styles.composer}>
    <UnsavedChangesGuard dirty={draft.lines.length > 0 || locked} />
    <DishCatalogue locked={locked} onObserved={observed} onAdd={(dish) => {
      try { edit(addBasketDish(draft, dish)) } catch (error) { setNotice(error instanceof Error ? error.message : 'Не удалось добавить блюдо.') }
    }} />
    <section className={styles.basket} aria-label="Корзина заказа"><h2>Корзина</h2><p className="text-caption text-muted">До 100 блюд, от 1 до 1000 целых порций каждого блюда. Доступность каталога относится к одному блюду отдельно.</p>
      {!draft.lines.length && <EmptyState title="Корзина пуста" description="Добавьте блюда из каталога или рекомендаций." />}
      <ul className={styles.lines}>{draft.lines.map((line) => <li className="ui-panel" key={line.dish.id}>
        <h3>{line.dish.name}</h3><p>Принятая рецептура: {line.dish.recipe_version ?? 'отсутствует'}</p>
        <details><summary>Принятый состав: {line.dish.name}</summary><ul>{line.dish.recipe.map((item) => <li key={item.product_id}>{item.product_name} — {formatQuantity(item.quantity_per_portion, item.unit)} на порцию</li>)}</ul></details>
        <PortionsField line={line} locked={locked} onChange={(portions) => edit({ lines: draft.lines.map((item) => item.dish.id === line.dish.id ? { ...item, portions } : item) })} />
        <Button variant="ghost" disabled={locked} aria-label={`Удалить ${line.dish.name}`} onClick={() => edit({ lines: draft.lines.filter((item) => item.dish.id !== line.dish.id) })}>Удалить блюдо</Button>
      </li>)}</ul>
      {needsReview && <section className="ui-panel" aria-label="Изменение рецептуры"><h3>Подтвердите актуальный состав</h3>
        <p>Рецептура или активность блюда изменились. Предыдущая проверка неактуальна.</p>
        {draft.lines.filter((line) => latest[line.dish.id] && (reviewRequired || recipeChanged(line.dish, latest[line.dish.id]))).map((line) => {
          const dish = latest[line.dish.id]
          return <div key={dish.id}><h4>{dish.name}: версия {line.dish.recipe_version ?? 'нет'} → {dish.recipe_version ?? 'нет'}</h4><p>{dish.is_active ? 'Блюдо активно' : 'Блюдо отключено'}</p><ul>{dish.recipe.map((item) => <li key={item.product_id}>{item.product_name} — {formatQuantity(item.quantity_per_portion, item.unit)} на порцию</li>)}</ul></div>
        })}
        <Button variant="secondary" loading={dishQuery.isFetching} onClick={() => { void refreshRecipes(draft.lines.map((line) => line.dish.id)) }}>Обновить рецептуры</Button>
        <Button disabled={locked || dishQuery.isFetching || reviewRequired && !reviewReady || !draft.lines.every((line) => latest[line.dish.id])} onClick={() => {
          edit({ lines: draft.lines.map((line) => ({ ...line, dish: basketDishSchema.parse(latest[line.dish.id]) })) }); setReviewRequired(false)
        }}>Подтверждаю актуальный состав</Button>
      </section>}
      {notice && <p className={styles.notice} role="alert">{notice}</p>}
      {refusal && <OrderRequirements result={refusal} />}
      {!locked && !needsReview && !blocked && current?.data && <><p role="status">{current.data.can_create ? 'Корзина проверена: можно оформить.' : 'Корзину нельзя оформить.'}</p><OrderRequirements result={current.data} /></>}
      {!locked && !needsReview && !blocked && current?.error && <ErrorState title="Не удалось проверить корзину" description={`${current.error.message} Проверка не является сохранённым отказом заказа.`} onRetry={() => setRevision((value) => value + 1)} />}
      {!locked && !!body && !needsReview && !blocked && !current?.data && !current?.error && <p role="status">{current?.loading ? 'Проверяем всю корзину…' : 'Ожидаем проверку корзины…'}</p>}
      {!body && draft.lines.length > 0 && <p role="alert">Проверьте целые порции, активность и рецептуры блюд.</p>}
      <p className="text-caption text-muted">Проверка не резервирует продукты. Сервер повторно проверит остатки при оформлении.</p>
      {locked ? <Button loading={mutation.isLoading} loadingLabel="Оформляем заказ…" onClick={() => { void send() }}>Повторить исходный запрос</Button> : <>
        <Button variant="secondary" disabled={!body || needsReview} onClick={() => { setBlocked(false); setRevision((value) => value + 1); setRefusal(undefined); setNotice(null) }}>Проверить ещё раз</Button>
        <Button disabled={!canCreate} onClick={() => { void send() }}>Оформить заказ</Button>
      </>}
    </section>
  </div>
}
function PortionsField({ line, locked, onChange }: { line: OrderDraft['lines'][number]; locked: boolean; onChange: (value: string) => void }) {
  const form = useForm({ values: { portions: line.portions }, mode: 'onChange', resolver: zodResolver(z.object({ portions: portionsSchema })) })
  const error = portionsSchema.safeParse(line.portions)
  return <FormField label={`Порции: ${line.dish.name}`} required error={error.success ? undefined : error.error.issues[0].message}>{(props) => <Input {...props} {...form.register('portions', { onChange: (event) => onChange(event.target.value) })} disabled={locked} inputMode="numeric" autoComplete="off" maxLength={20} />}</FormField>
}
