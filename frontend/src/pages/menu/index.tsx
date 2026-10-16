import { useEffect, useRef, useState } from 'react'
import { useSelector, useStore } from 'react-redux'
import { Link, useNavigate, useSearchParams, useLocation } from 'react-router'
import { defaultDishSearch, readDishSearch, dishSearchParams, changeDishSearch, useAdminDishesQuery, useLazyDishQuery, useUpdateHotOfferMutation, useLazyHotOfferPreferenceQuery, type Dish } from '@/entities/dish'
import { selectSession, saveWorkDraft, type SessionState } from '@/entities/session'
import { addBasketDish } from '@/features/compose-order'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Badge, Button, EmptyState, ErrorState, FormField, Input, Pagination, Select, Skeleton } from '@/shared/ui'
import styles from './Menu.module.css'
export function MenuPage() {
  const session = useSelector(selectSession), store = useStore<{ session: SessionState }>(), navigate = useNavigate(), [params,setParams] = useSearchParams()
  const values = { ...readDishSearch(params), is_active: 'true' as const }
  const query = useAdminDishesQuery(values, { refetchOnMountOrArgChange: true }), [load] = useLazyDishQuery(), [publish, publishing] = useUpdateHotOfferMutation(), [preference,readingPreference] = useLazyHotOfferPreferenceQuery()
  const [search,setSearch] = useState(values.search), [notice,setNotice] = useState(''), [busy,setBusy] = useState(''), lock = useRef(false)
  const location = useLocation(), currentLocation = useRef(location.key), mounted = useRef(true)
  useEffect(() => { currentLocation.current = location.key }, [location.key])
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const locked = !!session.drafts.order?.order?.attempt
  const management = session.user?.role === 'admin' || session.user?.role === 'manager'
  async function add(item: Dish) {
    if (lock.current) return
    lock.current = true; setBusy(item.id); setNotice('')
    const owner = session.user?.id, originalLocation = location.key
    try {
      const dish = await load(item.id, false).unwrap(), current = selectSession(store.getState())
      if (!mounted.current || currentLocation.current !== originalLocation || current.status !== 'authenticated' || current.user?.id !== owner) return
      if (!dish.is_active || !dish.is_available || dish.recipe_version !== item.recipe_version) throw new Error('Доступность или рецепт изменились. Обновите меню перед добавлением.')
      const draft = current.draftOwner === owner ? current.drafts.order?.order ?? { lines: [] } : { lines: [] }
      store.dispatch(saveWorkDraft({ key: 'order', draft: { order: addBasketDish(draft,dish,1) } }))
      void navigate('/orders/new')
    } catch (error) { if (mounted.current) setNotice(error instanceof Error ? error.message : 'Не удалось проверить блюдо. Попробуйте ещё раз.') }
    finally { lock.current = false; if (mounted.current) setBusy('') }
  }
  return <section className={styles.page}>
    <header className={styles.header}><div><h1>Меню</h1><p className="text-muted">Блюда, состав и доступные порции по текущему складу.</p></div><Link to="/recommendations/offers" className="ui-button ui-button--secondary">Горячие предложения</Link></header>
    <form className={styles.filters} onSubmit={(event) => { event.preventDefault(); setParams(dishSearchParams(changeDishSearch(values,{ search }))) }}>
      <FormField label="Найти блюдо">{(props) => <Input {...props} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Название блюда" />}</FormField>
      <FormField label="Доступность">{(props) => <Select {...props} value={values.availability} onChange={(event) => setParams(dishSearchParams(changeDishSearch(values,{ availability: event.target.value as typeof values.availability })))}><option value="all">Все блюда</option><option value="available">Можно приготовить</option><option value="unavailable">В стоп-листе</option></Select>}</FormField>
      <Button type="submit">Найти</Button><Button variant="ghost" onClick={() => { setSearch(''); setParams(dishSearchParams(defaultDishSearch)) }}>Сбросить</Button>
    </form>
    {locked && <p role="status">Перед добавлением блюда выясните результат исходного заказа.</p>}
    {notice && <p role="status">{notice}</p>}
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton rows={4} label="Загрузка меню" />}
      {query.error && <ErrorState title="Не удалось загрузить меню" onRetry={() => { void query.refetch() }} />}
      {query.currentData && <><p className="text-muted">Блюд: {query.currentData.total}</p><div className={styles.grid}>{query.currentData.items.map((item) => <article className={styles.card} key={item.id}>
        <header className={styles.header}><h2>{item.name}</h2><Badge tone={item.is_available ? 'success' : 'danger'}>{item.is_available ? 'Можно приготовить' : 'В стоп-листе'}</Badge></header>
        {item.description && <p>{item.description}</p>}<p>Доступно: <strong>{item.available_portions} порц.</strong></p>
        <details><summary>Состав на одну порцию</summary><ul>{item.recipe.map((line) => <li key={line.product_id}>{line.product_name} — {formatQuantity(line.quantity_per_portion,line.unit)}</li>)}</ul></details>
        <div className="ui-actions"><Button disabled={locked || !item.is_available || !!busy || !!query.error || query.isFetching} loading={busy === item.id} onClick={() => { void add(item) }}>Добавить в заказ</Button>
          {management && <Button variant="secondary" disabled={!item.is_available || publishing.isLoading || readingPreference.isFetching} onClick={() => {
            void preference(item.id,false).unwrap().then((current) => publish({ id: item.id, is_published: true, is_manual: true, message: current.message, expected_revision: current.revision }).unwrap()).then(() => setNotice(`${item.name}: добавлено в горячие предложения`)).catch((error: { status?: number }) => setNotice(error.status === 409 ? 'Настройки предложения изменились. Обновите меню и повторите публикацию.' : 'Не удалось опубликовать предложение.'))
          }}>В горячие предложения</Button>}
        </div>
      </article>)}</div>{!query.currentData.items.length && <EmptyState title="Блюда не найдены" description="Измените поиск или фильтр доступности." />}
      {query.currentData.total > values.page_size && <Pagination page={values.page} pageCount={Math.max(values.page,Math.ceil(query.currentData.total/values.page_size))} onPageChange={(page) => setParams(dishSearchParams(changeDishSearch(values,{ page })))} />}</>}
    </ApiAccessBoundary>
  </section>
}
