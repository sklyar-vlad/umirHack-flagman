import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { movementNames, type MovementSearch } from '@/entities/stock-movement'
import { historyFilterSchema, historyInstantToInput, convertHistoryTimezone } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './MovementsPage.module.css'

export function MovementFilters({ values, timezone, onChange, onReset }: { values: MovementSearch; timezone: string; onChange: (patch: Partial<MovementSearch>) => void; onReset: () => void }) {
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const identifier = z.string().trim().refine((value) => !value || z.uuid().safeParse(value).success, 'Укажите UUID')
  const schema = historyFilterSchema(draftZone, values).safeExtend({ product_id: identifier, batch_id: identifier, order_id: identifier, type: z.enum(['', 'receipt', 'order_consumption', 'manual_writeoff', 'expiration']) })
  const { register, handleSubmit, getValues, reset, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { search: '', from: historyInstantToInput(values.from, timezone), to: historyInstantToInput(values.to, timezone), product_id: values.product_id, batch_id: values.batch_id, order_id: values.order_id, type: values.type as z.input<typeof schema>['type'] } })
  return <form noValidate aria-label="Фильтры движений" className={styles.filters} onSubmit={handleSubmit(({ from, to, product_id, batch_id, order_id, type }) => { if (draftZone === timezone) onChange({ from, to, product_id, batch_id, order_id, type }) })}>
    <FormField label="Тип движения">{(props) => <Select {...props} {...register('type')}><option value="">Все движения</option>{Object.entries(movementNames).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select>}</FormField>
    <FormField label="Создано с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создано до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    <details className={styles.wide} open={!!(values.product_id || values.batch_id || values.order_id)}><summary>Фильтры по продукту, партии и заказу</summary><div className={styles.filters}>
      <FormField label="ID продукта" error={errors.product_id?.message}>{(props) => <Input {...props} {...register('product_id')} />}</FormField>
      <FormField label="ID партии" error={errors.batch_id?.message}>{(props) => <Input {...props} {...register('batch_id')} />}</FormField>
      <FormField label="ID заказа" error={errors.order_id?.message}>{(props) => <Input {...props} {...register('order_id')} />}</FormField>
    </div></details>
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className={styles.wide}>Часовой пояс: {draftZone}. Сначала новые движения; верхняя граница периода не включается.</p>
    {draftZone !== timezone && <div className={styles.wide} role="status"><p>Зона ресторана изменилась. Пересмотрите введённый период.</p><Button variant="secondary" onClick={() => { try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) } catch { setZoneError(true) } }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Проверьте даты: период нельзя однозначно преобразовать.</p>}
  </form>
}
