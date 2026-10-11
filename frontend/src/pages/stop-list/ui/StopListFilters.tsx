import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { StopListSearch } from '@/entities/dish'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './StopListPage.module.css'

const schema = z.object({ search: z.string() })
export function StopListFilters({ values, onChange, onReset }: { values: StopListSearch; onChange: (patch: Partial<StopListSearch>) => void; onReset: () => void }) {
  const { register, handleSubmit } = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { search: values.search } })
  return <form className={styles.filters} onSubmit={handleSubmit(onChange)} aria-label="Поиск в стоп-листе">
    <FormField label="Название блюда">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
    <div className={styles.actions}><Button type="submit">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить поиск</Button></div>
    <FormField label="Блюд на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
  </form>
}
