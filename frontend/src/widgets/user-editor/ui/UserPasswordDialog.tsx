import { useEffect, useRef, useState } from 'react'
import { useSelector } from 'react-redux'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { userPasswordSchema, type User } from '@/entities/user'
import type { SessionState } from '@/entities/session'
import { useAuth } from '@/features/auth'
import { Button, Dialog, FormField, Input, Skeleton } from '@/shared/ui'
import { useUserWrite, userWriteMessage, unknownUserWrite } from './useUserWrite'
const schema = z.object({ password: userPasswordSchema })
export function UserPasswordDialog({ user, onReload, onDirty, onPending, onClose, onSaved, returnFocusRef }: { user: User; onReload: () => Promise<User>; onDirty: (value: boolean) => void; onPending: (value: boolean) => void; onClose: () => void; onSaved: (user: User) => void; returnFocusRef: { current: HTMLElement | null } }) {
  const auth = useAuth(), session = useSelector((state: { session: SessionState }) => state.session), writer = useUserWrite()
  const [baseline, setBaseline] = useState(user), [fresh, setFresh] = useState<User>(), [reading, setReading] = useState(false), [readFailed, setReadFailed] = useState(false), [discard, setDiscard] = useState(false)
  const stay = useRef<HTMLButtonElement>(null)
  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm<{ password: string }>({ resolver: zodResolver(schema), defaultValues: { password: '' } })
  useEffect(() => { onPending(writer.busy || reading); return () => onPending(false) }, [onPending, writer.busy, reading])
  const blocked = writer.busy || reading || !!fresh || readFailed || writer.error?.code === 'REVISION_CONFLICT' || unknownUserWrite(writer.error) || [403, 404].includes(Number(writer.error?.status))
  const close = () => { if (writer.busy || reading) return; if (isDirty) setDiscard(true); else { reset({ password: '' }); onDirty(false); onClose() } }
  const submit = async ({ password }: { password: string }) => {
    if (blocked) return
    const result = await writer.send({ kind: 'password', id: baseline.id, body: { password, expected_revision: baseline.revision } })
    if (!result) return
    reset({ password: '' }); onDirty(false); onSaved(result); onClose()
    if (result.id.toLowerCase() === session.user?.id.toLowerCase()) auth.expire()
  }
  return <Dialog open title="Сброс пароля" description={`Пользователь: ${baseline.name}. Все его сессии будут отозваны. ${baseline.id.toLowerCase() === session.user?.id.toLowerCase() ? 'Это ваша учётная запись: потребуется новый вход.' : 'Пользователю потребуется войти с новым паролем.'}`} onClose={close} returnFocusRef={returnFocusRef}>
    <form noValidate aria-label="Сброс пароля" onSubmit={(event) => { void handleSubmit(submit)(event) }}>
      <p>Ревизия: {baseline.revision}. Новый пароль: 12–128 символов. Email-письмо не отправляется.</p>
      <FormField label="Новый пароль" error={errors.password?.message}>{(props) => <Input {...props} {...register('password', { onChange: () => onDirty(true) })} type="password" autoComplete="new-password" disabled={writer.busy || reading} aria-required />}</FormField>
      {writer.error && <p role="alert">{userWriteMessage(writer.error)}</p>}
      <div className="ui-actions"><Button variant="secondary" disabled={writer.busy || reading} onClick={close}>Отмена</Button><Button type="submit" variant="danger" loading={writer.busy} disabled={blocked}>Сменить пароль и отозвать сессии</Button></div>
    </form>
    {(writer.error?.code === 'REVISION_CONFLICT' || unknownUserWrite(writer.error) || readFailed) && <Button disabled={reading || writer.busy} onClick={() => {
      setReading(true); setReadFailed(false)
      void onReload().then(setFresh).catch(() => setReadFailed(true)).finally(() => setReading(false))
    }}>Прочитать актуального пользователя</Button>}
    {reading && <Skeleton label="Чтение актуальной ревизии пользователя" />}
    {readFailed && <p role="alert">Не удалось прочитать актуальную ревизию. Повторите чтение.</p>}
    {fresh && <section aria-label="Актуальная ревизия пароля"><p>Актуальная ревизия: {fresh.revision}. Пользователь: {fresh.name}, {fresh.email}. Проверьте результат предыдущей попытки перед новой сменой пароля.</p><Button onClick={() => { setBaseline(fresh); setFresh(undefined); writer.clearError() }}>Продолжить с этой ревизией</Button></section>}
    <Dialog open={discard} title="Закрыть без смены пароля?" onClose={() => setDiscard(false)} initialFocusRef={stay} actions={<><Button ref={stay} variant="secondary" onClick={() => setDiscard(false)}>Остаться</Button><Button variant="danger" onClick={() => { reset({ password: '' }); onDirty(false); onClose() }}>Закрыть и очистить пароль</Button></>}><p>Введённый пароль будет удалён.</p></Dialog>
  </Dialog>
}
