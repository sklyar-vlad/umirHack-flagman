import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { createCategorySchema, useCreateCategoryMutation, useUpdateCategoryMutation, type Category } from '@/entities/product-category'
import { isApiError } from '@/shared/api'
import { Button, Dialog, FormField, Input } from '@/shared/ui'
export function CategoryForm({ category, onSaved, onCancel, onDirty, onReload }: { category?: Category; onSaved: (category: Category) => void; onCancel: () => void; onDirty: (dirty: boolean) => void; onReload: () => void }) {
  const [create, created] = useCreateCategoryMutation(), [update, updated] = useUpdateCategoryMutation()
  const [discard, setDiscard] = useState(false)
  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm({ resolver: zodResolver(createCategorySchema), defaultValues: { name: category?.name ?? '' } })
  const mutation = category ? updated : created, error = isApiError(mutation.error) ? mutation.error : undefined
  const busy = mutation.isLoading, conflict = error?.code === 'REVISION_CONFLICT', denied = error?.status === 403
  const maxRevision = category && category.revision > 2147483646
  useEffect(() => { onDirty(isDirty || busy) }, [isDirty, busy, onDirty])
  const close = () => { reset(); onDirty(false); onCancel() }
  return <>
    <form aria-label={category ? 'Переименование категории' : 'Создание категории'} onSubmit={handleSubmit(async (body) => {
      const result = category ? await update({ id: category.id, body: { ...body, expected_revision: category.revision } }) : await create(body)
      if (result.data) { reset({ name: result.data.name }); onDirty(false); onSaved(result.data) }
    })}>
      <h2>{category ? `Переименовать: ${category.name}` : 'Новая категория'}</h2>
      <p>Название от 1 до 200 символов; уникальность проверяет сервер.{category && ` Ревизия: ${category.revision}.`}</p>
      <FormField label="Название категории" error={errors.name?.message} required>{(props) => <Input {...props} {...register('name')} autoFocus disabled={busy || denied} />}</FormField>
      {error && <p role="alert">{denied ? 'Недостаточно прав для изменения категории.' : conflict ? 'Категория изменена другим пользователем. Загрузите актуальную версию и проверьте новое имя перед сохранением.' : error.code === 'CONFLICT' ? 'Категория с таким названием уже существует.' : error.status === 'NETWORK_ERROR' || error.status === 'TIMEOUT_ERROR' || error.status === 'INVALID_RESPONSE' || typeof error.status === 'number' && error.status >= 500 ? 'Результат сохранения неизвестен. Проверьте список перед новой отправкой: изменение могло сохраниться.' : 'Не удалось сохранить категорию. Проверьте название и повторите попытку.'}</p>}
      {maxRevision && <p role="alert">Ревизия достигла предела изменения, обратитесь к администратору системы.</p>}
      <div className="ui-actions"><Button type="submit" loading={busy} loadingLabel="Сохранение категории…" disabled={conflict || denied || !!maxRevision || !!category && !isDirty}>Сохранить категорию</Button><Button variant="secondary" disabled={busy} onClick={() => isDirty ? setDiscard(true) : close()}>Отмена</Button>{conflict && <Button variant="secondary" onClick={() => { onDirty(false); onReload() }}>Загрузить актуальную категорию</Button>}</div>
    </form>
    <Dialog open={discard} title="Отменить изменения категории?" onClose={() => setDiscard(false)} actions={<><Button variant="secondary" onClick={() => setDiscard(false)}>Продолжить редактирование</Button><Button variant="danger" onClick={close}>Отменить изменения</Button></>}><p>Несохранённое название будет потеряно.</p></Dialog>
  </>
}
