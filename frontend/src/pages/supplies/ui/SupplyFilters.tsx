import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { supplyFilterSchema, supplyInstantToInput, type SupplySearch } from '@/entities/supply'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './SuppliesPage.module.css'

export function SupplyFilters({ values, timezone, onChange, onReset }: { values: SupplySearch; timezone: string; onChange: (patch: Partial<SupplySearch>) => void; onReset: () => void }) {
  const schema = supplyFilterSchema(timezone, values)
  const { register, handleSubmit, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema), defaultValues: { search: values.search, from: supplyInstantToInput(values.from, timezone), to: supplyInstantToInput(values.to, timezone) },
  })
  return <form noValidate className={styles.filters} onSubmit={handleSubmit(onChange)} aria-label="Поиск и период поставок">
    <FormField label="Поиск по номеру">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
    <FormField label="Создана с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создана до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    <div className={styles.actions}><Button type="submit">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <p className="text-caption text-muted">Период по времени создания записи, часовой пояс: {timezone}. Верхняя граница не входит в период. Сначала новые поставки.</p>
    <FormField label="Поставок на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>
      {[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}
    </Select>}</FormField>
  </form>
}
