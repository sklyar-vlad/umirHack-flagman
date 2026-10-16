import { convertHistoryTimezone } from '@/shared/lib/history'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { supplyFilterSchema, supplyInstantToInput, type SupplySearch } from '@/entities/supply'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './SuppliesPage.module.css'

export function SupplyFilters({ values, timezone, onChange, onReset }: { values: SupplySearch; timezone: string; onChange: (patch: Partial<SupplySearch>) => void; onReset: () => void }) {
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const schema = supplyFilterSchema(draftZone, values)
  const { register, handleSubmit, getValues, reset, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema), defaultValues: { search: values.search, from: supplyInstantToInput(values.from, draftZone), to: supplyInstantToInput(values.to, draftZone) },
  })
  return <form noValidate className={styles.filters} onSubmit={handleSubmit((patch) => { if (draftZone === timezone) onChange(patch) })} aria-label="Поиск и период поставок">
    {draftZone !== timezone && <div role="status"><p>Зона ресторана изменена на {timezone}. Введённые даты остаются в {draftZone}. Пересмотрите их перед поиском.</p><Button variant="secondary" onClick={() => {
      try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) }
      catch { setZoneError(true); void handleSubmit(() => {})() }
    }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Дата некорректна или неоднозначна в новой зоне. Исходные даты сохранены; проверьте период.</p>}
    <FormField label="Поиск по номеру">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
    <FormField label="Создана с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создана до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <p className="text-caption text-muted">Период по времени создания записи, часовой пояс ввода: {draftZone}. Верхняя граница не входит в период. Сначала новые поставки.</p>
    <FormField label="Поставок на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>
      {[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}
    </Select>}</FormField>
  </form>
}
