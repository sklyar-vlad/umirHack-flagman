import { useForm } from 'react-hook-form'
import { useProductCategoriesQuery } from '@/entities/inventory'
import type { ProductSearch } from '@/entities/product'
import { isApiError } from '@/shared/api'
import { Button, ErrorState, FormField, Input, Select } from '@/shared/ui'
import styles from './AdminProducts.module.css'
export function ProductFilters({ values, onChange, onReset }: { values: ProductSearch; onChange: (patch: Partial<ProductSearch>) => void; onReset: () => void }) {
  const categories = useProductCategoriesQuery(undefined, { refetchOnMountOrArgChange: true })
  const { register, handleSubmit } = useForm({ defaultValues: { search: values.search } })
  return <div className={styles.filters}>
    <form className={styles.actions} role="search" aria-label="Поиск продуктов" onSubmit={handleSubmit(({ search }) => onChange({ search: search.trim() }))}>
      <FormField label="Поиск продукта">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
      <Button type="submit" variant="secondary">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button>
    </form>
    <div><FormField label="Категория">{(props) => <Select {...props} value={values.category_id} disabled={categories.isFetching || !!categories.error} onChange={(event) => onChange({ category_id: event.target.value })}>
      <option value="">Все категории</option>
      {values.category_id && !categories.currentData?.some((category) => category.id === values.category_id) && <option value={values.category_id}>Категория по ссылке</option>}
      {!categories.error && categories.currentData?.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}
    </Select>}</FormField>
    {categories.isFetching && <p role="status">Загрузка категорий…</p>}
    {isApiError(categories.error) && categories.error.status === 403 ? <p role="alert">Нет доступа к категориям.</p> : categories.error && <ErrorState title="Не удалось загрузить категории" onRetry={() => { void categories.refetch() }} retrying={categories.isFetching} />}</div>
    <FormField label="Активность">{(props) => <Select {...props} value={values.is_active} onChange={(event) => onChange({ is_active: event.target.value as ProductSearch['is_active'] })}><option value="all">Все продукты</option><option value="true">Активные</option><option value="false">Отключённые</option></Select>}</FormField>
    <FormField label="Продуктов на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
  </div>
}
