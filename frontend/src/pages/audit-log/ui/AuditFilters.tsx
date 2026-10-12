import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { auditActions, auditEntities, type AuditSearch } from '@/entities/audit-log'
import { convertHistoryTimezone, historyFilterSchema, historyInstantToInput } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './AuditPage.module.css'

export function AuditFilters({ values, timezone, onChange, onReset }: { values: AuditSearch; timezone: string; onChange: (patch: Partial<AuditSearch>) => void; onReset: () => void }) {
  const [advanced, setAdvanced] = useState(!!(values.actor_id || values.action || values.entity_type || values.entity_id || values.from || values.to))
  const [draftZone, setDraftZone] = useState(timezone), [zoneError, setZoneError] = useState(false)
  const schema = historyFilterSchema(draftZone, values).safeExtend({
    actor_id: z.string().trim().refine((value) => !value || value === 'system' || z.uuid().safeParse(value).success, 'Укажите UUID пользователя или system'),
    entity_id: z.string().trim().refine((value) => !value || z.uuid().safeParse(value).success, 'Укажите UUID объекта'),
    action: z.enum(['', 'create', 'update', 'password_change', 'manual_writeoff', 'expiration']),
    entity_type: z.enum(['', 'product_category', 'product', 'dish', 'supply', 'order', 'stock_movement', 'user', 'settings']),
  })
  const { register, handleSubmit, reset, getValues, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { search: values.search, from: historyInstantToInput(values.from, timezone), to: historyInstantToInput(values.to, timezone), actor_id: values.actor_id, entity_id: values.entity_id, action: values.action as z.input<typeof schema>['action'], entity_type: values.entity_type as z.input<typeof schema>['entity_type'] } })
  return <form noValidate aria-label="Фильтры журнала" className={styles.filters} onSubmit={handleSubmit((patch) => { if (draftZone === timezone) onChange(patch) })}>
    <div className={styles.search}><FormField label="Поиск по действию, пользователю или объекту">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField></div>
    <details className={styles.search} open={advanced} onToggle={(event) => setAdvanced(event.currentTarget.open)}><summary>Фильтры по автору, объекту и периоду</summary><div className={styles.advanced}>
    <FormField label="Автор (UUID или system)" error={errors.actor_id?.message}>{(props) => <Input {...props} {...register('actor_id')} />}</FormField>
    <FormField label="Действие" error={errors.action?.message}>{(props) => <Select {...props} {...register('action')}><option value="">Все действия</option>{Object.entries(auditActions).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select>}</FormField>
    <FormField label="Тип объекта" error={errors.entity_type?.message}>{(props) => <Select {...props} {...register('entity_type')}><option value="">Все типы</option>{Object.entries(auditEntities).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select>}</FormField>
    <FormField label="ID объекта" error={errors.entity_id?.message}>{(props) => <Input {...props} {...register('entity_id')} />}</FormField>
    <FormField label="Создано с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создано до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    </div></details>
    <div className={styles.actions}><Button type="submit" disabled={draftZone !== timezone}>Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className={styles.search}>Часовой пояс: {draftZone}. Сначала новые записи. Верхняя граница периода не включается.</p>
    {draftZone !== timezone && <div className={styles.search} role="status"><p>Зона ресторана изменена на {timezone}. Пересмотрите введённый период перед поиском.</p><Button variant="secondary" onClick={() => { try { reset(convertHistoryTimezone(getValues(), values, draftZone, timezone)); setDraftZone(timezone); setZoneError(false) } catch { setZoneError(true) } }}>Пересмотреть период в новой зоне</Button></div>}
    {zoneError && <p role="alert">Проверьте даты: период нельзя однозначно преобразовать в новую зону.</p>}
  </form>
}
