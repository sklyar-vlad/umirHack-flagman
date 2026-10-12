import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { orderReasons, type AttemptSearch, type OrderTab } from '@/entities/order'
import { convertHistoryTimezone, historyFilterSchema, historyInstantToInput } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './OrdersPage.module.css'

export function OrderFilters({ values, tab, timezone, onChange, onReset }: { values: AttemptSearch; tab: OrderTab; timezone: string; onChange: (patch: Partial<AttemptSearch>) => void; onReset: () => void }) {
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const schema = historyFilterSchema(draftZone, values).safeExtend({ reason_code: z.enum(['', 'INSUFFICIENT_STOCK', 'RECIPE_CHANGED', 'DISH_UNAVAILABLE']) })
  const { register, handleSubmit, getValues, reset, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema), defaultValues: { search: values.search, from: historyInstantToInput(values.from, draftZone), to: historyInstantToInput(values.to, draftZone), reason_code: tab === 'attempts' ? values.reason_code : '' },
  })
  return <form noValidate className={styles.filters} onSubmit={handleSubmit((patch) => { if (draftZone === timezone) onChange(patch) })} aria-label="Поиск и период заказов">
    {draftZone !== timezone && <div role="status"><p>Зона ресторана изменена на {timezone}. Введённые даты остаются в {draftZone}. Пересмотрите их перед поиском.</p><Button variant="secondary" onClick={() => {
      try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) }
      catch { setZoneError(true); void handleSubmit(() => {})() }
    }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Дата некорректна или неоднозначна в новой зоне. Исходные даты сохранены; проверьте период.</p>}
    <FormField label={tab === 'orders' ? 'Номер заказа или название блюда' : 'ID попытки или название блюда'}>{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
    <FormField label="Создана с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создана до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    {tab === 'attempts' && <FormField label="Причина отказа" error={errors.reason_code?.message}>{(props) => <Select {...props} {...register('reason_code')}><option value="">Все причины</option>{Object.entries(orderReasons).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select>}</FormField>}
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className="text-caption text-muted">Время создания записи, часовой пояс ввода: {draftZone}. Верхняя граница не входит в период. Сначала новые записи.</p>
  </form>
}
