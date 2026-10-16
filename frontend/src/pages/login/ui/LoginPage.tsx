import { useEffect } from 'react'
import { useSelector } from 'react-redux'
import type { SessionState } from '@/entities/session'
import { useAuth } from '@/features/auth'
import { LoginForm } from '@/features/auth/login'
import { Button, Skeleton } from '@/shared/ui'
import styles from './LoginPage.module.css'

export function LoginPage() {
  const auth = useAuth()
  const status = useSelector((state: { session: SessionState }) => state.session.status)
  useEffect(() => { if (status === 'unknown') void auth.restore() }, [auth, status])
  return <main className={styles.page}><section className={`ui-panel ${styles.panel}`} aria-labelledby="login-title">
    <img src="/logo-wordmark.svg" alt="Кладовая" width="200" />
    <h1 id="login-title">Вход в Кладовую</h1>
    <p className="text-muted">Войдите, чтобы продолжить работу с ресторанным складом.</p>
    {status === 'unknown' || status === 'restoring' ? <Skeleton label="Проверяем сессию" /> : <>
      {status === 'error' && <Button variant="secondary" onClick={() => { void auth.restore() }}>Повторить проверку сессии</Button>}
      <LoginForm />
    </>}
  </section></main>
}
