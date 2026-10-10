import { isRouteErrorResponse, Link, useRouteError } from 'react-router'

export function RouteError() {
  const error = useRouteError()
  const notFound = isRouteErrorResponse(error) && error.status === 404

  return (
    <main className="app-placeholder ui-panel">
      <h1>{notFound ? 'Страница не найдена' : 'Не удалось открыть страницу'}</h1>
      <p className="text-muted">{notFound ? 'Проверьте адрес или вернитесь на главную.' : 'Попробуйте открыть приложение ещё раз.'}</p>
      <Link className="ui-button ui-button--secondary" to="/">На главную</Link>
    </main>
  )
}
