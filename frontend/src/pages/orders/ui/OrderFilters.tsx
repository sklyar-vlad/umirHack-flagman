import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { orderReasons, type AttemptSearch, type OrderTab } from '@/entities/order'
import { historyFilterSchema, historyInstantToInput } from '@/shared/lib/history'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './OrdersPage.module.css'

export function OrderFilters({ values, tab, timezone, onChange, onReset }: { values: AttemptSearch; tab: OrderTab; timezone: string; onChange: (patch: Partial<AttemptSearch>) => void; onReset: () => void }) {
  const schema = historyFilterSchema(timezone, values).safeExtend({ reason_code: z.enum(['', 'INSUFFICIENT_STOCK', 'RECIPE_CHANGED', 'DISH_UNAVAILABLE']) })
  const { register, handleSubmit, formState: { errors } } = useForm<z.input<typeof schema>, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema), defaultValues: { search: values.search, from: historyInstantToInput(values.from, timezone), to: historyInstantToInput(values.to, timezone), reason_code: tab === 'attempts' ? values.reason_code : '' },
  })
  return <form noValidate className={styles.filters} onSubmit={handleSubmit(onChange)} aria-label="Поиск и период заказов">
    <FormField label={tab === 'orders' ? 'Номер заказа или название блюда' : 'ID попытки или название блюда'}>{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
    <FormField label="Создана с (включительно)" error={errors.from?.message}>{(props) => <Input {...props} {...register('from')} type="datetime-local" step="any" />}</FormField>
    <FormField label="Создана до (не включая)" error={errors.to?.message}>{(props) => <Input {...props} {...register('to')} type="datetime-local" step="any" />}</FormField>
    {tab === 'attempts' && <FormField label="Причина отказа" error={errors.reason_code?.message}>{(props) => <Select {...props} {...register('reason_code')}><option value="">Все причины</option>{Object.entries(orderReasons).map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select>}</FormField>}
    <div className={styles.actions}><Button type="submit">Найти</Button><Button variant="ghost" onClick={onReset}>Сбросить фильтры</Button></div>
    <FormField label="Записей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
    <p className="text-caption text-muted">Время создания записи, часовой пояс: {timezone}. Верхняя граница не входит в период. Сначала новые записи.</p>
  </form>
}
