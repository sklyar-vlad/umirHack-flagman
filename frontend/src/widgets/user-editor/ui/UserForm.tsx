import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useRolesQuery, type User } from '@/entities/user'
import { replaceSession, type SessionState } from '@/entities/session'
import { userFormSchema, newUserFormSchema, emptyUser, userValues, userPatch, revokesAccess, type NewUserValues, type UserValues } from '@/features/manage-user'
import { useAuth } from '@/features/auth'
import { UnsavedChangesGuard } from '@/features/unsaved-changes'
import { Button, Checkbox, Dialog, ErrorState, FormField, Input, Select, Skeleton } from '@/shared/ui'
import { isApiError, type ApiError } from '@/shared/api'
import { useUserWrite, userWriteMessage, unknownUserWrite } from './useUserWrite'
import { UserPasswordDialog } from './UserPasswordDialog'
import styles from './UserForm.module.css'
export function UserForm({ user, password = false, onReload }: { user?: User; password?: boolean; onReload?: () => Promise<User> }) {
  const auth = useAuth(), dispatch = useDispatch(), session = useSelector((state: { session: SessionState }) => state.session)
  const latestSession = useRef(session)
  useEffect(() => { latestSession.current = session }, [session])
  const [baseline, setBaseline] = useState(user), [saved, setSaved] = useState<User>(), [confirm, setConfirm] = useState<UserValues>()
  const [fresh, setFresh] = useState<User>(), [reading, setReading] = useState(false), [readError, setReadError] = useState<ApiError>()
  const [passwordOpen, setPasswordOpen] = useState(password), [passwordDirty, setPasswordDirty] = useState(false), [passwordBusy, setPasswordBusy] = useState(false), [notice, setNotice] = useState('')
  const writer = useUserWrite(), initiator = useRef<HTMLButtonElement>(null), stay = useRef<HTMLButtonElement>(null)
  const roles = useRolesQuery(undefined, { refetchOnMountOrArgChange: true })
  const { register, handleSubmit, reset, setValue, setError, watch, formState: { errors, isDirty } } = useForm<NewUserValues>({
    resolver: zodResolver(user ? userFormSchema.extend({ initial_password: z.string() }) : newUserFormSchema),
    defaultValues: { ...(user ? userValues(user) : emptyUser), initial_password: '' },
  })
  useEffect(() => {
    if (writer.error?.status !== 422) return
    for (const field of writer.error.details.fields ?? []) {
      const path = field.path === 'password' ? 'initial_password' : field.path
      if (['name', 'email', 'role', 'is_active', 'initial_password'].includes(path)) setError(path as keyof NewUserValues, { type: 'server', message: 'Проверьте значение поля.' }, { shouldFocus: true })
    }
  }, [writer.error, setError])
  const blocked = writer.busy || reading || !!fresh || !!readError || unknownUserWrite(writer.error) || ['REVISION_CONFLICT'].includes(writer.error?.code ?? '') || [403, 404].includes(Number(writer.error?.status)) || roles.isFetching || !!roles.error || !roles.currentData
  const dirty = !saved && (isDirty || passwordDirty || writer.busy || passwordBusy)
  const submit = async (values: UserValues) => {
    if (blocked || passwordOpen || confirm || !roles.currentData?.items.some((role) => role.code === values.role)) return
    if (baseline && Object.keys(userPatch(values, baseline)).length === 1) return
    if (baseline && (revokesAccess(values, baseline) || values.is_active !== baseline.is_active)) { setConfirm(values); return }
    await save(values)
  }
  const save = async (values: UserValues) => {
    const initialPassword = baseline ? undefined : (values as NewUserValues).initial_password
    const body = baseline ? { kind: 'update' as const, id: baseline.id, body: userPatch(values, baseline) } : { kind: 'create' as const, body: { ...userValuesForWrite(values), password: initialPassword! } }
    const result = await writer.send(body)
    if (!result) { setConfirm(undefined); return }
    setValue('initial_password', ''); reset({ ...userValues(result), initial_password: '' }); setConfirm(undefined)
    if (result.id.toLowerCase() === latestSession.current.user?.id.toLowerCase()) {
      if (baseline && revokesAccess(values, baseline)) { auth.expire(); return }
      dispatch(replaceSession({ ...latestSession.current, user: { id: result.id, name: result.name, email: result.email, role: result.role } }))
    }
    setSaved(result)
  }
  const reload = async () => {
    if (!onReload || reading || writer.busy) return
    setReading(true); setReadError(undefined)
    try { setFresh(await onReload()) } catch (error) { setReadError(isApiError(error) ? error : { status: 'NETWORK_ERROR', code: 'NETWORK_ERROR', message: 'Ошибка чтения', details: {} }) }
    finally { setReading(false) }
  }
  const displayError = writer.error
  if (saved) return <section className={styles.form}><h2>Пользователь сохранён: {saved.name}</h2><p role="status">Ревизия: {saved.revision}. Список пользователей обновлён.</p><p>Пароль не сохраняется в приложении. Email-приглашение не отправляется.</p></section>
  return <section className={styles.form} aria-label="Форма пользователя">
    <UnsavedChangesGuard dirty={dirty} pending={writer.busy || passwordBusy} />
    {notice && <p role="status">{notice}</p>}
    {roles.isFetching && <Skeleton label="Загрузка справочника ролей" />}
    {roles.error && <ErrorState title={isApiError(roles.error) && roles.error.status === 403 ? 'Недостаточно прав' : 'Не удалось загрузить роли'} onRetry={() => { void roles.refetch() }} />}
    <form noValidate aria-label={user ? 'Редактирование пользователя' : 'Создание пользователя'} onSubmit={(event) => { void handleSubmit(submit)(event) }}>
      <p>{baseline ? `Ревизия: ${baseline.revision}. Пароль меняется отдельно.` : 'Создайте пользователя с начальным паролем. Email-приглашение не отправляется.'}</p>
      <fieldset disabled={writer.busy || reading || !!confirm || passwordOpen || writer.error?.status === 403 || isApiError(roles.error) && roles.error.status === 403} className={styles.fields}>
        <div className={styles.grid}>
          <FormField label="Имя пользователя" error={errors.name?.message}>{(props) => <Input {...props} {...register('name')} aria-required autoComplete="off" />}</FormField>
          <FormField label="Email пользователя" error={errors.email?.message}>{(props) => <Input {...props} {...register('email')} type="email" aria-required autoComplete="off" />}</FormField>
        </div>
        <FormField label="Роль пользователя" error={errors.role?.message}>{(props) => <Select {...props} {...register('role')} value={watch('role')} disabled={roles.isFetching || !!roles.error} aria-required>{roles.currentData?.items.map((role) => <option key={role.code} value={role.code}>{role.name}</option>)}</Select>}</FormField>
        <Checkbox {...register('is_active')}>Пользователь активен</Checkbox>
        {!user && <FormField label="Начальный пароль" error={errors.initial_password?.message} hint="От 12 до 128 символов. Пароль не сохраняется в черновик.">{(props) => <Input {...props} {...register('initial_password')} type="password" autoComplete="new-password" aria-required />}</FormField>}
        <Button type="submit" disabled={blocked} loading={writer.busy}>Сохранить пользователя</Button>
      </fieldset>
    </form>
    {displayError && <div role="alert"><p>{userWriteMessage(displayError)}</p>{displayError.status === 422 && displayError.details.fields?.map((field) => <p key={field.path}>{field.path === 'password' ? 'Пароль' : field.path}: проверьте значение поля.</p>)}</div>}
    {baseline && (writer.error?.code === 'REVISION_CONFLICT' || unknownUserWrite(writer.error) || readError) && <Button variant="secondary" disabled={reading || writer.busy} onClick={() => { void reload() }}>Прочитать актуального пользователя</Button>}
    {reading && <Skeleton label="Чтение актуальной ревизии пользователя" />}
    {readError && <p role="alert">{readError.status === 403 ? 'Недостаточно прав для чтения пользователя.' : readError.status === 404 ? 'Пользователь не найден.' : 'Не удалось прочитать актуальную ревизию. Повторите чтение.'}</p>}
    {fresh && <section className={styles.review} aria-label="Актуальная ревизия"><h2>Проверьте актуального пользователя</h2><p>Ревизия: {fresh.revision}. Имя: {fresh.name}. Email: {fresh.email}. Роль: {roles.currentData?.items.find((role) => role.code === fresh.role)?.name}. Активность: {fresh.is_active ? 'Активен' : 'Отключён'}.</p><p>Введённые значения сохранены. Сравните их перед новой отправкой.</p><Button variant="secondary" onClick={() => { setBaseline(fresh); setFresh(undefined); writer.clearError() }}>Продолжить с этой ревизией</Button></section>}
    {baseline && <Button ref={initiator} variant="secondary" disabled={writer.busy || reading || !!fresh || !!readError || unknownUserWrite(writer.error) || writer.error?.code === 'REVISION_CONFLICT' || !!confirm} onClick={() => setPasswordOpen(true)}>Сбросить пароль</Button>}
    <Dialog open={!!confirm} title="Подтвердите изменение доступа" onClose={() => { if (!writer.busy) setConfirm(undefined) }} initialFocusRef={stay} actions={<><Button ref={stay} variant="secondary" disabled={writer.busy} onClick={() => setConfirm(undefined)}>Отмена</Button><Button variant="danger" disabled={writer.busy} onClick={() => { if (confirm) void save(confirm) }}>Изменить доступ</Button></>}><p>Смена роли или отключение отзовёт все сессии пользователя. Повторное включение их не восстановит.{baseline?.id.toLowerCase() === session.user?.id.toLowerCase() && ' Это ваша учётная запись: потребуется новый вход.'}</p>{confirm && writer.error && <p role="alert">{userWriteMessage(writer.error)}</p>}</Dialog>
    {passwordOpen && baseline && onReload && <UserPasswordDialog user={baseline} onReload={onReload} onDirty={setPasswordDirty} onPending={setPasswordBusy} returnFocusRef={initiator} onClose={() => { setPasswordOpen(false); setPasswordDirty(false) }} onSaved={(result) => { setBaseline(result); writer.clearError(); setNotice('Пароль изменён. Все сессии пользователя отозваны.') }} />}
  </section>
}
function userValuesForWrite(values: UserValues) { return { name: values.name, email: values.email, role: values.role, is_active: values.is_active } }
