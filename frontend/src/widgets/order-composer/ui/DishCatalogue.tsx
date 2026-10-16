import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useDishesQuery, type Dish } from '@/entities/dish'
import { ApiAccessBoundary } from '@/features/access'
import { formatQuantity } from '@/shared/lib/quantity'
import { Badge, Button, EmptyState, ErrorState, FormField, Input, Pagination, Skeleton } from '@/shared/ui'
import styles from './OrderComposer.module.css'

const searchSchema = z.object({ search: z.string().max(200, 'Не больше 200 символов.') })
export function DishCatalogue({ locked, onAdd, onObserved }: { locked: boolean; onAdd: (dish: Dish) => void; onObserved: (dish: Dish) => void }) {
  const [args, setArgs] = useState({ search: '', page: 1 })
  const catalogue = useDishesQuery(args, { refetchOnMountOrArgChange: true })
  const form = useForm({ defaultValues: { search: '' }, resolver: zodResolver(searchSchema) })
  const data = catalogue.currentData
  useEffect(() => { if (!catalogue.error) data?.items.forEach(onObserved) }, [data, catalogue.error, onObserved])
  return <section className={styles.catalogue} aria-label="Каталог блюд"><h2>Каталог блюд</h2>
    <form className={styles.search} noValidate onSubmit={form.handleSubmit(({ search }) => setArgs({ search: search.trim(), page: 1 }))}>
      <FormField label="Поиск блюда" error={form.formState.errors.search?.message}>{(props) => <Input {...props} {...form.register('search')} type="search" />}</FormField><Button type="submit" variant="secondary">Найти блюдо</Button>
    </form>
    {catalogue.isFetching && !catalogue.currentData && <Skeleton label="Загрузка блюд" rows={3} />}
    <ApiAccessBoundary error={catalogue.error}>
      {catalogue.error && <ErrorState title="Не удалось загрузить блюда" description="Корзина сохранена. Обновите каталог перед добавлением блюда." onRetry={() => { void catalogue.refetch() }} retrying={catalogue.isFetching} />}
      {!catalogue.isFetching && !catalogue.error && data && !data.items.length && <EmptyState title={args.search ? 'Блюда не найдены' : 'Каталог пуст'} description="Измените поиск или повторите загрузку." />}
      {!catalogue.error && <ul className={styles.dishes}>{data?.items.map((dish) => <li className="ui-panel" key={dish.id}>
        <h3>{dish.name}</h3>{dish.description && <p>{dish.description}</p>}
        <Badge tone={dish.is_available ? 'success' : 'warning'}>{!dish.is_active ? 'Отключено' : dish.is_available ? 'Доступно' : 'Недоступно'}</Badge>
        <p className="text-caption">Отдельно доступно: {dish.available_portions} порц. Рецептура: {dish.recipe_version ?? 'отсутствует'}.</p>
        <details onToggle={(event) => { if (event.currentTarget.open) onObserved(dish) }}><summary>Состав: {dish.name}</summary><ul>{dish.recipe.map((item) => <li key={item.product_id}>{item.product_name} — {formatQuantity(item.quantity_per_portion, item.unit)} на порцию</li>)}</ul>{!dish.recipe.length && <p>Рецептура отсутствует.</p>}</details>
        <Button disabled={locked || catalogue.isFetching || !dish.is_active || !dish.recipe_version} onClick={() => { onObserved(dish); onAdd(dish) }} aria-label={`Добавить ${dish.name}`}>Добавить в корзину</Button>
      </li>)}</ul>}
      {data && data.total > 20 && <Pagination page={args.page} pageCount={Math.ceil(data.total / 20)} loading={catalogue.isFetching} onPageChange={(page) => setArgs({ ...args, page })} />}
    </ApiAccessBoundary>
  </section>
}
