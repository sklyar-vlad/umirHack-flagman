import { useState } from 'react'
import { Link } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { Badge, Button, Checkbox, DataTable, Dialog, Drawer, EmptyState, ErrorState, FormField, Icon, Input, NoResults, Notification, Pagination, Select, Skeleton, Switch, Textarea, type DataColumn } from '@/shared/ui'
import styles from './UiKitPage.module.css'

const schema = z.object({ name: z.string().trim().min(2, 'Введите не менее двух символов').max(120, 'Не более 120 символов'), category: z.string().min(1, 'Выберите категорию'), note: z.string().max(500, 'Не более 500 символов'), agreed: z.boolean().refine(Boolean, 'Подтвердите условия'), enabled: z.boolean() })
type DemoForm = z.infer<typeof schema>
const defaults: DemoForm = { name: '', category: '', note: '', agreed: false, enabled: true }
const records = [
  { id: '1', name: 'Моцарелла для пиццы и горячих блюд ресторанной кухни', quantity: '12,500 кг', status: 'В наличии' },
  { id: '2', name: 'Оливковое масло первого холодного отжима', quantity: '8 л', status: 'Заканчивается' },
  { id: '3', name: 'Контейнеры для доставки с герметичной крышкой', quantity: '120 шт', status: 'В наличии' },
]
const columns: DataColumn<(typeof records)[number]>[] = [
  { key: 'name', header: 'Название продукта', render: (row) => row.name },
  { key: 'quantity', header: 'Остаток', render: (row) => row.quantity },
  { key: 'status', header: 'Состояние', render: (row) => <Badge tone={row.status === 'В наличии' ? 'success' : 'warning'}>{row.status}</Badge> },
]

export function UiKitPage() {
  const [modal, setModal] = useState<'dialog' | 'drawer' | null>(null)
  const [notification, setNotification] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [page, setPage] = useState(1)
  const [retrying, setRetrying] = useState(false)
  const [recovered, setRecovered] = useState(false)
  const [query, setQuery] = useState('несуществующий продукт')
  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm<DemoForm>({ resolver: zodResolver(schema), defaultValues: defaults })
  const fields = { name: register('name'), category: register('category'), note: register('note'), agreed: register('agreed'), enabled: register('enabled') }
  const save = async (values: DemoForm) => {
    setSubmitting(true)
    await new Promise<void>((resolve) => { setTimeout(resolve, 600) })
    reset(values)
    setSubmitting(false)
    setNotification(true)
  }
  return <main className={styles.page}>
    <header className={styles.header}><img src="/logo-wordmark.svg" alt="Кладовая" width="180" /><Link to="/">← На главную</Link><h1>Компоненты интерфейса</h1><p className="text-muted">Локальный стенд разработки. Примеры не сохраняются на сервере.</p></header>
    <UnsavedChangesGuard dirty={isDirty} />
    <section className="ui-panel"><h2>Кнопки и статусы</h2><div className="ui-actions">
      <Button onClick={() => setModal('dialog')}><Icon name="plus" />Открыть диалог</Button>
      <Button variant="secondary" onClick={() => setModal('drawer')}>Открыть боковую панель</Button>
      <Button variant="danger" onClick={() => setNotification(true)}>Показать уведомление</Button>
      <Button variant="ghost">Дополнительное действие</Button><Button disabled>Недоступно</Button><Button loading loadingLabel="Загрузка…">Загрузка</Button>
    </div><div className="ui-actions"><Badge>Черновик</Badge><Badge tone="success">В наличии</Badge><Badge tone="warning">Заканчивается</Badge><Badge tone="danger">Просрочено</Badge></div></section>
    <section className="ui-panel"><h2>Поля и валидация</h2><form className={styles.form} noValidate onSubmit={handleSubmit(save)}>
      <fieldset disabled={submitting} className={styles.fields}>
        <FormField label="Название продукта с длинной русской подписью для проверки переноса" required error={errors.name?.message} hint="От 2 до 120 символов">{(props) => <Input {...fields.name} {...props} required autoComplete="off" />}</FormField>
        <FormField label="Категория" required error={errors.category?.message}>{(props) => <Select {...fields.category} {...props} required><option value="">Выберите категорию</option><option value="dairy">Молочные продукты</option><option value="grocery">Бакалея</option></Select>}</FormField>
        <FormField label="Комментарий" error={errors.note?.message}>{(props) => <Textarea {...fields.note} {...props} placeholder="Дополнительная информация для сотрудников склада" />}</FormField>
        <div className="ui-field"><Checkbox {...fields.agreed} aria-invalid={errors.agreed ? true : undefined} aria-describedby={errors.agreed ? 'agreement-error' : undefined}>Подтверждаю условия хранения и проверки срока годности</Checkbox>{errors.agreed && <p id="agreement-error" role="alert" className="ui-field-error">{errors.agreed.message}</p>}</div>
        <Switch {...fields.enabled}>Показывать уведомления об остатках</Switch>
        <FormField label="Недоступное поле">{(props) => <Input {...props} disabled value="Значение недоступно для изменения" />}</FormField>
      </fieldset>
      <div className="ui-actions"><Button type="submit" loading={submitting} loadingLabel="Сохраняем…">Сохранить пример</Button><Button variant="secondary" disabled={submitting} onClick={() => reset(defaults)}>Сбросить форму</Button></div><p className="text-caption text-muted">{isDirty ? 'Есть несохранённые изменения' : 'Нет несохранённых изменений'}</p>
    </form></section>
    {notification && <Notification title="Пример сохранён локально" onDismiss={() => setNotification(false)}>Это демонстрация уведомления; бизнес-данные не изменены.</Notification>}
    <section className="ui-panel"><DataTable rows={records.slice(page - 1, page)} columns={columns} rowKey={(row) => row.id} caption="Пример складских остатков" emptyState={<EmptyState />} /><Pagination page={page} pageCount={records.length} onPageChange={setPage} /></section>
    <section className="ui-panel"><h2>Состояния экрана</h2><div className={styles.states}><Skeleton /><EmptyState /><NoResults query={query} onReset={() => setQuery('')} />{recovered ? <Notification title="Данные снова доступны" /> : <ErrorState retrying={retrying} onRetry={() => { setRetrying(true); setTimeout(() => { setRetrying(false); setRecovered(true) }, 600) }} />}</div></section>
    <Dialog open={modal === 'dialog'} onClose={() => setModal(null)} title="Подтверждение действия с длинным названием" description="Проверьте введённые данные перед продолжением." actions={<Button variant="secondary" onClick={() => setModal(null)}>Готово</Button>}><p>Диалог поддерживает клавиатуру, Escape и возврат фокуса к кнопке открытия.</p></Dialog>
    <Drawer open={modal === 'drawer'} onClose={() => setModal(null)} title="Параметры отображения" actions={<Button onClick={() => setModal(null)}>Применить</Button>}><FormField label="Сортировка">{(props) => <Select {...props}><option>По названию продукта</option><option>По остатку</option></Select>}</FormField><Checkbox defaultChecked>Показывать продукты без остатка</Checkbox></Drawer>
  </main>
}
