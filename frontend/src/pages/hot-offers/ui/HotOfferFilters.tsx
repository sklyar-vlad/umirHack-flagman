import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { HotOfferSearch } from '@/entities/dish'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './HotOffersPage.module.css'
const schema = z.object({ search: z.string().max(200, 'Не больше 200 символов.') })
export function HotOfferFilters({ values, onChange, onReset }: { values: HotOfferSearch; onChange: (patch: Partial<HotOfferSearch>) => void; onReset: () => void }) {
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { search: values.search } })
  return <div className={styles.filters}>
    <form className={styles.actions} role="search" aria-label="Поиск предложений" noValidate onSubmit={form.handleSubmit(({ search }) => onChange({ search: search.trim() }))}>
      <FormField label="Поиск блюда" error={form.formState.errors.search?.message}>{(props) => <Input {...props} {...form.register('search')} type="search" />}</FormField>
      <Button type="submit" variant="secondary">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button>
    </form>
    <FormField label="Блюд на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>
      {[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}
    </Select>}</FormField>
  </div>
}
