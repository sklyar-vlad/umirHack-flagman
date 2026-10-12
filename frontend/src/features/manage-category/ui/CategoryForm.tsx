import { useEffect, useImperativeHandle, useState, type Ref } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { createCategorySchema, useCreateCategoryMutation, useUpdateCategoryMutation, type Category } from '@/entities/product-category'
import { isApiError } from '@/shared/api'
import { Button, Dialog, FormField, Input } from '@/shared/ui'

export interface CategoryFormHandle { requestClose: () => void }
export function CategoryForm({ category, onSaved, onCancel, onDirty, onReload, ref, focusRef }: {
  category?: Category
  focusRef?: { current: HTMLInputElement | null }
  onSaved: (category: Category) => void
  onCancel: () => void
  onDirty: (dirty: boolean) => void
  onReload: () => Promise<Category | undefined>
  ref?: Ref<CategoryFormHandle>
}) {
  const [create, created] = useCreateCategoryMutation(), [update, updated] = useUpdateCategoryMutation()
  const [discard, setDiscard] = useState(false), [snapshot, setSnapshot] = useState(category)
  const [reloading, setReloading] = useState(false), [reloadFailed, setReloadFailed] = useState(false), [review, setReview] = useState(false)
  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm({ resolver: zodResolver(createCategorySchema), defaultValues: { name: category?.name ?? '' } })
  const nameField = register('name')
  const mutation = category ? updated : created, error = isApiError(mutation.error) ? mutation.error : undefined
  const busy = mutation.isLoading || reloading, conflict = error?.code === 'REVISION_CONFLICT', denied = error?.status === 403, missing = error?.status === 404
  const maxRevision = snapshot && snapshot.revision > 2147483646
  useEffect(() => { onDirty(isDirty || busy || review) }, [isDirty, busy, review, onDirty])
  const close = () => { reset(); onDirty(false); onCancel() }
  const requestClose = () => { if (!busy) { if (isDirty || review) setDiscard(true); else close() } }
  useImperativeHandle(ref, () => ({ requestClose }))
  const reload = async () => {
    setReloading(true); setReloadFailed(false)
    try {
      const fresh = await onReload()
      if (fresh) { setSnapshot(fresh); updated.reset(); setReview(true) } else setReloadFailed(true)
    } finally { setReloading(false) }
  }
  return <>
    <form aria-label={category ? 'Переименование категории' : 'Создание категории'} onSubmit={handleSubmit(async (body) => {
      // Keyboard submits follow the same revision/access lock as the button.
      if (busy || conflict || denied || missing || review || maxRevision || category && !isDirty) return
      const result = snapshot ? await update({ id: snapshot.id, body: { ...body, expected_revision: snapshot.revision } }) : await create(body)
      if (result.data) { reset({ name: result.data.name }); onDirty(false); onSaved(result.data) }
    })}>
      <h2>{category ? `Переименовать: ${category.name}` : 'Новая категория'}</h2>
      <p>Название от 1 до 200 символов; уникальность проверяет сервер.{snapshot && ` Ревизия: ${snapshot.revision}.`}</p>
      <FormField label="Название категории" error={errors.name?.message} required>{(props) => <Input {...props} {...nameField} ref={(node) => { nameField.ref(node); if (focusRef) focusRef.current = node }} autoFocus disabled={busy || denied} />}</FormField>
      {error && <p role="alert">{denied ? 'Недостаточно прав для изменения категории.' : missing ? 'Категория больше не найдена. Введённое название сохранено; проверьте справочник.' : conflict ? 'Категория изменена другим пользователем. Загрузите актуальную версию и проверьте новое имя перед сохранением.' : error.code === 'CONFLICT' ? 'Категория с таким названием уже существует.' : error.status === 422 ? 'Сервер отклонил название категории. Проверьте его и повторите попытку.' : error.status === 'NETWORK_ERROR' || error.status === 'TIMEOUT_ERROR' || error.status === 'INVALID_RESPONSE' || typeof error.status === 'number' && error.status >= 500 ? 'Результат сохранения неизвестен. Проверьте список перед новой отправкой: изменение могло сохраниться.' : 'Не удалось сохранить категорию. Проверьте название и повторите попытку.'}</p>}
      {reloadFailed && <p role="alert">Не удалось загрузить актуальную категорию. Введённое название сохранено; повторите чтение.</p>}
      {review && snapshot && <div role="status"><p>Актуальное название: {snapshot.name}. Ревизия: {snapshot.revision}. Ваше название сохранено. Проверьте его перед повторной отправкой.</p><Button variant="secondary" onClick={() => setReview(false)}>Подтвердить актуальную ревизию</Button></div>}
      {maxRevision && <p role="alert">Ревизия достигла предела изменения, обратитесь к администратору системы.</p>}
      <div className="ui-actions"><Button type="submit" loading={mutation.isLoading} loadingLabel="Сохранение категории…" disabled={busy || conflict || denied || missing || review || !!maxRevision || !!category && !isDirty}>Сохранить категорию</Button><Button variant="secondary" disabled={busy} onClick={requestClose}>Отмена</Button>{(conflict || missing || reloadFailed) && <Button variant="secondary" disabled={busy} onClick={() => { void reload() }}>Загрузить актуальную категорию</Button>}</div>
    </form>
    <Dialog open={discard} title="Отменить изменения категории?" onClose={() => setDiscard(false)} actions={<><Button variant="secondary" onClick={() => setDiscard(false)}>Продолжить редактирование</Button><Button variant="danger" onClick={close}>Отменить изменения</Button></>}><p>Несохранённое название будет потеряно.</p></Dialog>
  </>
}
