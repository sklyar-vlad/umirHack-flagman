import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useSearchParams } from 'react-router'
import { useCategoriesQuery, type Category } from '@/entities/product-category'
import { readProductSearch } from '@/entities/product'
import { CategoryDialog } from '@/features/manage-category'
import { ApiAccessBoundary } from '@/features/access'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { isApiError } from '@/shared/api'
import { Button, EmptyState, ErrorState, FormField, Input, NoResults, Pagination, Skeleton } from '@/shared/ui'
import styles from './AdminProducts.module.css'
function CategorySearchForm({ value, onSearch }: { value: string; onSearch: (search: string) => void }) {
  const { register, handleSubmit } = useForm({ defaultValues: { search: value } })
  return <form role="search" aria-label="Поиск категорий" className={styles.actions} onSubmit={handleSubmit(({ search }) => onSearch(search.trim()))}><FormField label="Поиск категории">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField><Button type="submit" variant="secondary">Найти категории</Button></form>
}
export function CategoriesPage() {
  const [params, setParams] = useSearchParams(), { search, page, page_size } = readProductSearch(params)
  const query = useCategoriesQuery({ search, page, page_size, sort: 'name' }, { refetchOnMountOrArgChange: true }), data = query.currentData
  const denied = isApiError(query.error) && query.error.status === 403
  const [editing, setEditing] = useState<string | null>(null), [dirty, setDirty] = useState(false), [notice, setNotice] = useState('')
  const change = (value: { search?: string; page?: number }) => { const next = new URLSearchParams(); if (value.search ?? search) next.set('search', value.search ?? search); if ((value.page ?? 1) > 1) next.set('page', String(value.page)); if (page_size !== 50) next.set('page_size', String(page_size)); setParams(next) }
  const initiator = useRef<HTMLButtonElement | null>(null)
  const returnFocus = useRef(false), createButton = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    if (returnFocus.current && editing === null && !query.isFetching) {
      returnFocus.current = false
      const target = initiator.current?.isConnected ? initiator.current : createButton.current
      target?.focus()
    }
  }, [editing, query.isFetching])
  const close = () => { returnFocus.current = true; setDirty(false); setEditing(null) }
  const saved = (category: Category) => { close(); setNotice(`Сохранена категория: ${category.name}.`); void query.refetch() }
  return <div className={styles.page}>
    <UnsavedChangesGuard dirty={dirty} />
    <header className={styles.row}><h1>Категории продуктов</h1><div className={styles.actions}><Link className="ui-button ui-button--secondary" to="/admin/products">В каталог продуктов</Link><Button ref={createButton} disabled={editing !== null} onClick={(event) => { initiator.current = event.currentTarget; setEditing('new'); setNotice('') }}>Создать категорию</Button><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить категории</Button></div></header>
    <p className="text-muted">Переименование сохраняет привязки продуктов. Физическое удаление категории не предусмотрено.</p>
    {notice && <p role="status">{notice}</p>}
    {editing !== null && !denied && <CategoryDialog key={editing} id={editing === 'new' ? undefined : editing} onSaved={saved} onClose={close} onDirty={setDirty} returnFocusRef={initiator} actions={<Link className="ui-button ui-button--secondary" to="/admin/products">В каталог продуктов</Link>} />}
    <CategorySearchForm key={search} value={search} onSearch={(search) => change({ search })} />
    <ApiAccessBoundary error={query.error}>
      {query.isFetching && !query.currentData && <Skeleton label="Загрузка категорий" />}
      {query.error && <ErrorState title="Не удалось загрузить категории" description={data ? 'Показан последний список; данные могут быть устаревшими. Изменение недоступно до обновления.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status">Категорий: {data.total}. Страница {data.page}.</p><div className={styles.list}>{data.items.map((category) => <article className={styles.card} key={category.id} aria-label={`Категория: ${category.name}`}><h2>{category.name}</h2><p>Ревизия: {category.revision}</p><Button variant="secondary" disabled={editing !== null || query.isFetching || !!query.error} onClick={(event) => { initiator.current = event.currentTarget; setEditing(category.id); setNotice('') }} aria-label={`Переименовать: ${category.name}`}>Переименовать</Button></article>)}</div>
        {!data.items.length && (page > 1 ? <EmptyState title="На этой странице нет категорий" /> : search ? <NoResults query={search} onReset={() => change({ search: '' })} /> : <EmptyState title="Нет категорий" description="Создайте первую категорию для продуктов." />)}
        {data.total > page_size && <Pagination page={page} pageCount={Math.max(page, Math.ceil(data.total / page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
