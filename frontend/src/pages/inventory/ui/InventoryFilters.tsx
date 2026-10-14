import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useProductCategoriesQuery, type InventorySearch } from '@/entities/inventory'
import { ApiAccessBoundary } from '@/features/access'
import { Button, FormField, Input, Select, ErrorState } from '@/shared/ui'
import styles from './InventoryPage.module.css'

const searchSchema = z.object({ search: z.string() })
export function InventoryFilters({ values, onChange }: { values: InventorySearch; onChange: (patch: Partial<InventorySearch>) => void }) {
  const categories = useProductCategoriesQuery()
  const { register, handleSubmit } = useForm({ resolver: zodResolver(searchSchema), defaultValues: { search: values.search } })
  return <div className={styles.controls}>
    <form className={styles.search} onSubmit={handleSubmit(({ search }) => onChange({ search: search.trim() }))} role="search" aria-label="Поиск продуктов">
      <FormField label="Поиск продукта">{(props) => <Input {...props} {...register('search')} type="search" placeholder="Название продукта" />}</FormField>
      <Button type="submit" variant="secondary">Найти</Button>
    </form>
    <div>
      <FormField label="Категория">{(props) => <Select {...props} value={values.category_id} disabled={(categories.isFetching && !categories.currentData) || !!categories.error} onChange={(event) => onChange({ category_id: event.target.value })}>
        <option value="">Все категории</option>
        {values.category_id && !categories.currentData?.some((category) => category.id === values.category_id) && <option value={values.category_id}>Категория по ссылке</option>}
        {categories.currentData?.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}
      </Select>}</FormField>
      {categories.isFetching && <p role="status" className="text-caption">Загрузка категорий…</p>}
      <ApiAccessBoundary error={categories.error}>{categories.error && <ErrorState title="Не удалось загрузить категории" onRetry={() => { void categories.refetch() }} retrying={categories.isFetching} />}</ApiAccessBoundary>
    </div>
    <FormField label="Сортировка продуктов">{(props) => <Select {...props} value={values.sort} onChange={(event) => onChange({ sort: event.target.value === 'nearest_expires_at' ? 'nearest_expires_at' : 'name' })}>
      <option value="name">По названию</option><option value="nearest_expires_at">По ближайшему годному сроку</option>
    </Select>}</FormField>
    <FormField label="Продуктов на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>
      {[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}
    </Select>}</FormField>
  </div>
}
