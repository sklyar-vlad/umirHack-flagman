import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { ShortageSearch } from '@/entities/ingredient-shortage'
import { historyFilterSchema, historyInstantToInput, convertHistoryTimezone } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './IngredientShortagesPage.module.css'

export function ShortageFilters({ values, timezone, onChange, onReset }: { values: ShortageSearch; timezone: string; onChange: (patch: Partial<ShortageSearch>) => void; onReset: () => void }) {
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const schema = historyFilterSchema(draftZone, values)
  const { register, handleSubmit, getValues, reset, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { search: '', from: historyInstantToInput(values.from, timezone), to: historyInstantToInput(values.to, timezone) } })
  return <form noValidate aria-label="Фильтры недостачи" className={styles.filters} onSubmit={handleSubmit(({ from, to }) => { if (draftZone === timezone) onChange({ from, to }) })}>
    <FormField label="Отказы с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Отказы до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className={styles.wide}>Часовой пояс: {draftZone}. Приоритет по числу отказов задаёт сервер; верхняя граница периода не включается.</p>
    {draftZone !== timezone && <div className={styles.wide} role="status"><p>Зона ресторана изменилась. Пересмотрите введённый период.</p><Button variant="secondary" onClick={() => { try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) } catch { setZoneError(true) } }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Проверьте даты: период нельзя однозначно преобразовать.</p>}
  </form>
}
