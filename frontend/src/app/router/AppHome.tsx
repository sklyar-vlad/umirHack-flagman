import { Link } from 'react-router'

export function AppHome() {
  return (
    <main className="app-placeholder ui-panel">
      <img src="/logo-wordmark.svg" alt="Кладовая" width="200" />
      <h1>Ресторанный склад</h1>
      <p className="text-muted">Интерфейс приложения в разработке.</p>
      {import.meta.env.DEV && (
        <Link className="ui-button ui-button--secondary" to="/dev/api">Проверить подключение к API</Link>
      )}
    </main>
  )
}
