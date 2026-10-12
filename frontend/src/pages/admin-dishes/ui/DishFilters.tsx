import { useForm } from 'react-hook-form'
import type { DishSearch } from '@/entities/dish'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './AdminDishes.module.css'
export function DishFilters({ values, onChange, onReset }: { values: DishSearch; onChange: (patch: Partial<DishSearch>) => void; onReset: () => void }) {
  const { register, handleSubmit } = useForm({ defaultValues: { search: values.search } })
  return <div className={styles.filters}>
    <form className={styles.actions} role="search" aria-label="Поиск блюд" onSubmit={handleSubmit(({ search }) => onChange({ search: search.trim() }))}>
      <FormField label="Поиск блюда">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
      <Button type="submit" variant="secondary">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button>
    </form>
    <FormField label="Активность">{(props) => <Select {...props} value={values.is_active} onChange={(event) => onChange({ is_active: event.target.value as DishSearch['is_active'] })}><option value="all">Все блюда</option><option value="true">Активные</option><option value="false">Отключённые</option></Select>}</FormField>
    <FormField label="Доступность">{(props) => <Select {...props} value={values.availability} onChange={(event) => onChange({ availability: event.target.value as DishSearch['availability'] })}><option value="all">Любая доступность</option><option value="available">Доступные</option><option value="unavailable">Недоступные</option></Select>}</FormField>
    <FormField label="Блюд на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
  </div>
}
