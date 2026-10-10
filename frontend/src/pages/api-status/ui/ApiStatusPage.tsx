import { Link } from 'react-router'
import { isApiError, useHealthcheckQuery } from '@/shared/api'
import { env } from '@/shared/config'
import styles from './ApiStatusPage.module.css'

export function ApiStatusPage() {
  const { data, error, isLoading, isFetching, refetch } = useHealthcheckQuery()
  const apiError = isApiError(error) ? error : undefined

  return (
    <main className={styles.page}>
      <Link className={styles.back} to="/">← На главную</Link>
      <header className={styles.header}>
        <p className="text-caption text-muted">Инструмент разработчика</p>
        <h1>Подключение к API</h1>
        <p className="text-muted">{env.mockModeEnabled
          ? 'Проверка интерфейса на тестовых ответах. Реальный backend не вызывается.'
          : 'Проверка HTTP-процесса backend. Готовность базы и бизнес-операций проверяется отдельно.'}</p>
      </header>

      <section className={`ui-panel ${styles.panel}`} aria-labelledby="connection-title">
        <div className={styles.panelHeader}>
          <h2 id="connection-title">Healthcheck</h2>
          <button
            className="ui-button ui-button--secondary"
            type="button"
            disabled={isFetching}
            aria-busy={isFetching}
            onClick={() => { void refetch() }}
          >
            {isFetching ? 'Проверяем…' : error ? 'Повторить' : 'Проверить снова'}
          </button>
        </div>
        <p className={styles.endpoint}>GET /api/v1/healthcheck</p>
        <div aria-live="polite" aria-atomic="true">
          {isLoading && <p>Подключаемся к серверу…</p>}
          {isFetching && data && <p>Обновляем ответ сервера…</p>}
          {data && !isFetching && !error && <p className="ui-badge ui-badge--success">{env.mockModeEnabled ? 'Моковый ответ получен' : 'Backend отвечает'}</p>}
        </div>

        {error && (
          <div className={`ui-alert ui-alert--danger ${styles.error}`} role="alert">
            <p className="text-strong">{apiError?.message ?? 'Не удалось проверить подключение. Повторите запрос.'}</p>
            {apiError && (
              <dl className={styles.details}>
                <div><dt>Код</dt><dd>{apiError.code}</dd></div>
                {typeof apiError.status === 'number' && <div><dt>HTTP-статус</dt><dd>{apiError.status}</dd></div>}
                {apiError.request_id && <div><dt>Request ID</dt><dd>{apiError.request_id}</dd></div>}
                {apiError.retry_after && <div><dt>Retry-After</dt><dd>{apiError.retry_after}</dd></div>}
              </dl>
            )}
          </div>
        )}

        {data && (
          <div className={styles.result}>
            <p className="text-caption text-muted">{error ? 'Последний успешный ответ' : 'Ответ сервера'}</p>
            <dl className={styles.details}>
              <div><dt>Сервис</dt><dd>{data.service}</dd></div>
              <div><dt>Статус</dt><dd>{data.status}</dd></div>
            </dl>
          </div>
        )}
      </section>
    </main>
  )
}
