import { useEffect, useRef, useState } from 'react'
import { useSelector, useStore } from 'react-redux'
import { NavLink, useLocation, useNavigate, useSearchParams } from 'react-router'
import { changeHotOfferSearch, defaultHotOfferSearch, hotOfferSearchParams, readHotOfferSearch, useHotOffersQuery, useLazyDishQuery, type HotOffer, type HotOfferSearch } from '@/entities/dish'
import { saveWorkDraft, selectSession, visibleSections, canAccess, sectionForPath, type SessionState } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { addBasketDish } from '@/features/compose-order'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { HotOfferFilters } from './HotOfferFilters'
import { HotOfferCard } from './HotOfferCard'
import { DishCompositionDialog } from './DishCompositionDialog'
import styles from './HotOffersPage.module.css'
export function HotOffersPage() {
  const [params, setParams] = useSearchParams(), values = readHotOfferSearch(params), navigate = useNavigate()
  const session = useSelector(selectSession), store = useStore<{ session: SessionState }>()
  const timezone = session.settings?.timezone ?? 'Europe/Moscow'
  const query = useHotOffersQuery(values, { refetchOnMountOrArgChange: true, pollingInterval: 30_000, skipPollingIfUnfocused: true })
  const [loadDish] = useLazyDishQuery(), [selected, setSelected] = useState<{ offer: HotOffer; locationKey: string }>(), [adding, setAdding] = useState<string>(), [notice, setNotice] = useState<string>()
  const busy = useRef(false), mounted = useRef(true)
  const location = useLocation(), locationKey = useRef(location.key)
  useEffect(() => { locationKey.current = location.key }, [location.key])
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  const data = query.currentData
  const change = (patch: Partial<HotOfferSearch>) => { setNotice(undefined); setParams(hotOfferSearchParams(changeHotOfferSearch(values, patch))) }
  const reset = () => { setNotice(undefined); setParams(hotOfferSearchParams(defaultHotOfferSearch)) }
  async function add(offer: HotOffer) {
    if (busy.current) return
    busy.current = true; setAdding(offer.dish.id); setNotice(undefined)
    const owner = session.user?.id, originalLocation = location.key
    try {
      const fresh = await query.refetch().unwrap()
      const current = fresh.items.find((item) => item.dish.id === offer.dish.id)
      if (!current) throw new Error('Предложение больше не актуально: партия истекла, израсходована или изменились условия. Список обновлён.')
      const dish = await loadDish(offer.dish.id, false).unwrap()
      if (!dish.is_available || !dish.is_active || dish.recipe_version !== current.dish.recipe_version || current.dish.recipe_version !== offer.dish.recipe_version) throw new Error('Доступность или рецептура изменилась. Обновите предложения и просмотрите актуальный состав перед добавлением.')
      if (locationKey.current !== originalLocation) return
      const latest = selectSession(store.getState())
      if (!mounted.current || latest.status !== 'authenticated' || !latest.user || latest.user.id !== owner || !canAccess(latest.user.role, sectionForPath('/orders/new')!)) return
      const draft = latest.draftOwner === owner ? latest.drafts.order?.order ?? { lines: [] } : { lines: [] }
      const next = addBasketDish(draft, dish, 1)
      store.dispatch(saveWorkDraft({ key: 'order', draft: { order: next } }))
      void navigate('/orders/new')
    } catch (error) {
      if (mounted.current) setNotice(error instanceof Error ? error.message : 'Не удалось проверить предложение или загрузить состав. Корзина сохранена; повторите добавление после обновления.')
    } finally { busy.current = false; if (mounted.current) setAdding(undefined) }
  }
  const locked = session.draftOwner === session.user?.id && !!session.drafts.order?.order?.attempt
  return <div className={styles.page}>
    <h1>Рекомендации</h1>
    <nav aria-label="Вкладки рекомендаций" className={styles.actions}>{visibleSections(session.user!.role, 'recommendations').map((section) => <NavLink key={section.id} to={section.path} className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`}>{section.title}</NavLink>)}</nav>
    <header className={styles.row}><div><h2>Продвигать блюда</h2><p className="text-muted">Помогаем использовать реально затронутые истекающие партии.</p></div><Button variant="secondary" disabled={query.isFetching || !!adding} onClick={() => { void query.refetch() }}>Обновить предложения</Button></header>
    <p className="text-caption text-muted">Порядок, уровень и балл рассчитаны сервером. Одинаковые отображаемые баллы могут иметь разный ранг. Обновление каждые 30 секунд в активном окне.</p>
    <p className="ui-panel">Доступные порции относятся к каждому блюду отдельно. Предложения с общим ингредиентом не гарантируют возможность приготовить их вместе. Добавление одной порции открывает проверку всей корзины и не резервирует продукты.</p>
    {locked && <p role="status">Корзина заблокирована: сначала выясните результат исходного заказа.</p>}
    <HotOfferFilters key={values.search} values={values} onChange={change} onReset={reset} />
    {notice && <p role="alert">{notice}</p>}
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && <Skeleton label="Загрузка предложений" rows={3} />}
      {query.error && <ErrorState title="Не удалось загрузить предложения" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими; добавление отключено.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status" className="text-caption text-muted">Предложений: {data.total}. Страница {data.page}. Снимок: <time dateTime={data.meta.server_time}>{formatRestaurantDateTime(data.meta.server_time, timezone)}</time>.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((item) => <HotOfferCard key={item.dish.id} item={item} timezone={timezone} disabled={!!query.error || query.isFetching || !!adding || locked} adding={adding === item.dish.id} onComposition={() => setSelected({ offer: item, locationKey: location.key })} onAdd={() => { void add(item) }} />)}</div> : data.total > 0 || values.page > 1 ? <EmptyState title="На этой странице нет предложений" description="Вернитесь к первой странице списка." /> : values.search ? <NoResults query={values.search} onReset={reset} /> : <EmptyState title="Нет блюд для приоритетной продажи" description="Сейчас нет доступных активных блюд, одна порция которых использует истекающую партию. Это не означает, что склад пуст или все блюда недоступны." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
      {selected?.locationKey === location.key && <DishCompositionDialog key={selected.offer.dish.id} offer={selected.offer} present={!!data?.items.some((item) => item.dish.id === selected.offer.dish.id)} onClose={() => setSelected(undefined)} />}
    </ApiAccessBoundary>
  </div>
}
