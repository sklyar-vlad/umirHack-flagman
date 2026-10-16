import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useSelector } from 'react-redux'
import { useNavigate, useSearchParams } from 'react-router'
import { permittedReturnPath, type SessionState } from '@/entities/session'
import { isApiError } from '@/shared/api'
import { Button, FormField, Input } from '@/shared/ui'
import { authErrorMessage, type LoginCredentials } from '../api/draft-auth-adapter'
import { useAuth } from '../model/auth-context'
import { loginSchema, retryAfterDeadline } from '../model/login-schema'
import { safeReturnUrl } from '../model/return-url'
import styles from './LoginForm.module.css'

export function LoginForm() {
  const auth = useAuth()
  const session = useSelector((state: { session: SessionState }) => state.session)
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deadline, setDeadline] = useState(0)
  const [now, setNow] = useState(0)
  const errorRef = useRef<HTMLDivElement>(null)
  const { register, handleSubmit, resetField, formState: { errors, isSubmitting } } = useForm<LoginCredentials>({
    resolver: zodResolver(loginSchema), defaultValues: { email: '', password: '' },
  })
  const email = register('email')
  const password = register('password')
  const waiting = Math.max(0, Math.ceil((deadline - now) / 1000))
  const busy = isSubmitting || waiting > 0 || session.status === 'signing-out'
  useEffect(() => {
    if (!deadline) return
    const timer = setInterval(() => setNow(Date.now()), 250)
    return () => clearInterval(timer)
  }, [deadline])
  useEffect(() => { if (error) errorRef.current?.focus() }, [error])
  useEffect(() => {
    if (session.status === 'authenticated' && session.user) void navigate(permittedReturnPath(session.user.role, safeReturnUrl(params.get('returnTo'))), { replace: true })
  }, [navigate, params, session.status, session.user])
  async function submit(credentials: LoginCredentials) {
    setError(null)
    try { await auth.login(credentials) }
    catch (cause: unknown) {
      setError(isApiError(cause) ? authErrorMessage(cause) : 'Не удалось войти. Повторите попытку.')
      if (isApiError(cause) && cause.status === 429) {
        const timestamp = Date.now()
        setNow(timestamp)
        setDeadline(retryAfterDeadline(cause.retry_after, timestamp))
      }
    } finally { resetField('password'); setVisible(false) }
  }
  return <form className={styles.form} noValidate onSubmit={(event) => { if (busy) event.preventDefault(); else void handleSubmit(submit)(event) }}>
    {session.notice && <p role="status">{session.notice}</p>}
    {error && <div ref={errorRef} tabIndex={-1} role="alert" className={styles.error}>{error}</div>}
    <FormField label="Email" error={errors.email?.message} required>{(props) => <Input {...email} {...props} type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} disabled={busy} />}</FormField>
    <FormField label="Пароль" error={errors.password?.message} required>{(props) => <div className={styles.password}>
      <Input {...password} {...props} type={visible ? 'text' : 'password'} autoComplete="current-password" disabled={busy} />
      <Button variant="secondary" aria-controls={props.id} aria-pressed={visible} disabled={busy} onClick={() => setVisible((value) => !value)}>{visible ? 'Скрыть пароль' : 'Показать пароль'}</Button>
    </div>}</FormField>
    {waiting > 0 && <p role="status">Повторите вход через {waiting} сек.</p>}
    <Button type="submit" loading={isSubmitting} loadingLabel="Входим…" disabled={busy}>Войти</Button>
  </form>
}
