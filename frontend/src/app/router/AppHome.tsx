import { useSelector } from 'react-redux'
import type { SessionState } from '@/entities/session'
import { useAuth } from '@/features/auth'
import { Button } from '@/shared/ui'
import { Link } from 'react-router'

export function AppHome() {
  const auth = useAuth()
  const user = useSelector((state: { session: SessionState }) => state.session.user)
  return (
    <main className="app-placeholder ui-panel">
      <img src="/logo-wordmark.svg" alt="Кладовая" width="200" />
      <h1>Ресторанный склад</h1>
      <p>Вы вошли как {user?.name}.</p>
      <Button variant="secondary" onClick={() => { void auth.logout() }}>Выйти</Button>
      <p className="text-muted">Интерфейс приложения в разработке.</p>
      {import.meta.env.DEV && (
        <>
          <Link className="ui-button ui-button--secondary" to="/dev/api">Проверить подключение к API</Link>
          <Link className="ui-button ui-button--secondary" to="/dev/ui">Компоненты интерфейса</Link>
        </>
      )}
    </main>
  )
}
