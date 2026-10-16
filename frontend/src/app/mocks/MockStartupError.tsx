export function MockStartupError() {
  return (
    <main className="app-placeholder ui-panel" role="alert">
      <h1>Не удалось запустить моковый режим</h1>
      <p>Проверьте установку MSW, файл public/mockServiceWorker.js и VITE_MOCK_SCENARIO. Подробности доступны в консоли браузера.</p>
      <p>Приложение остановлено до первого API-запроса. Для работы с backend явно отключите VITE_ENABLE_MOCKS и перезапустите Vite.</p>
      <button className="ui-button ui-button--secondary" type="button" onClick={() => window.location.reload()}>
        Повторить запуск
      </button>
    </main>
  )
}
