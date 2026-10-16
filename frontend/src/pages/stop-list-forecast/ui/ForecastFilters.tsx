import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { ForecastSearch } from '@/entities/stop-list-forecast'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './ForecastPage.module.css'

const integer = (max: number, message: string) => z.string().regex(/^\d+$/, message).transform(Number).pipe(z.number().int().min(1, message).max(max, message))
const schema = z.object({ lookback_days: integer(90, 'Укажите целое число от 1 до 90.'), horizon_hours: integer(720, 'Укажите целое число от 1 до 720.') })
export function ForecastFilters({ values, onChange, onReset }: { values: ForecastSearch; onChange: (patch: Partial<ForecastSearch>) => void; onReset: () => void }) {
  const { register, handleSubmit, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { lookback_days: Number.isFinite(values.lookback_days) ? String(values.lookback_days) : '', horizon_hours: Number.isFinite(values.horizon_hours) ? String(values.horizon_hours) : '' } })
  return <form noValidate aria-label="Настройки прогноза" className={styles.filters} onSubmit={handleSubmit(onChange)}>
    <FormField label="История расхода, дней" error={errors.lookback_days?.message}>{(props) => <Input {...props} {...register('lookback_days')} type="number" min={1} max={90} step={1} />}</FormField>
    <FormField label="Горизонт прогноза, часов" error={errors.horizon_hours?.message}>{(props) => <Input {...props} {...register('horizon_hours')} type="number" min={1} max={720} step={1} />}</FormField>
    <div className="ui-actions"><Button type="submit">Применить</Button><Button variant="ghost" onClick={onReset}>Сбросить настройки</Button></div>
    <FormField label="Блюд на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, ...(Number.isInteger(values.page_size) && values.page_size >= 1 && values.page_size <= 100 ? [values.page_size] : [])])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
  </form>
}
