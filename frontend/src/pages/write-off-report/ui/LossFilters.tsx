import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { lossReasonSchema, type LossSearch } from '@/entities/write-off-report'
import { historyFilterSchema, historyInstantToInput, convertHistoryTimezone } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './LossPage.module.css'

export function LossFilters({ values, timezone, onChange, onReset }: { values: LossSearch; timezone: string; onChange: (patch: Partial<LossSearch>) => void; onReset: () => void }) {
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const identifier = z.string().trim().refine((value) => !value || z.uuid().safeParse(value).success, 'Укажите UUID')
  const schema = historyFilterSchema(draftZone, values).safeExtend({ product_id: identifier, reason: lossReasonSchema })
  const { register, handleSubmit, getValues, reset, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { search: '', from: historyInstantToInput(values.from, timezone), to: historyInstantToInput(values.to, timezone), product_id: values.product_id, reason: values.reason as z.input<typeof schema>['reason'] } })
  return <form noValidate aria-label="Фильтры потерь" className={styles.filters} onSubmit={handleSubmit(({ from, to, product_id, reason }) => { if (draftZone === timezone) onChange({ from, to, product_id, reason }) })}>
    <FormField label="Причина списания">{(props) => <Select {...props} {...register('reason')}><option value="all">Все потери</option><option value="expiration">Просрочка</option><option value="manual_writeoff">Ручные списания</option></Select>}</FormField>
    <FormField label="Потери с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Потери до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    <FormField label="ID продукта" error={errors.product_id?.message}>{(props) => <Input {...props} {...register('product_id')} placeholder="Все продукты" />}</FormField>
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className={styles.wide}>Часовой пояс: {draftZone}. Порядок по имени задаёт сервер; верхняя граница периода не включается.</p>
    {draftZone !== timezone && <div className={styles.wide} role="status"><p>Зона ресторана изменилась. Пересмотрите введённый период.</p><Button variant="secondary" onClick={() => { try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) } catch { setZoneError(true) } }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Проверьте даты: период нельзя однозначно преобразовать.</p>}
  </form>
}
