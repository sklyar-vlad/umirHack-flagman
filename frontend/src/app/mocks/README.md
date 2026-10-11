# Моковый режим

MSW 3 перехватывает запросы на уровне браузера. `main.tsx` динамически загружает `browser.ts` только при `import.meta.env.DEV && env.mockModeEnabled`, ожидает `startMocking()` и затем запускает React. Ошибка запуска не переключает приложение на реальный backend.

## Файлы

| Файл | Назначение |
|---|---|
| `browser.ts` | Создание worker, начальный сценарий, запуск и очистка при HMR |
| `handlers.ts` | Маршруты MSW; порядок от конкретных handlers к общему отказу |
| `responses.ts` | Healthcheck и ошибки по DTO backend; Request ID генерируется на каждый ответ |
| `scenarios.ts` | Допустимые сценарии и текущее значение в памяти |
| `MockModeBanner.tsx` | Явная индикация режима и переключение сценария |
| `MockStartupError.tsx` | Ошибка до запуска приложения и первого API-запроса |

## Добавление сценариев

Новые handlers добавляются перед общими `/api/v1/*` и `/api/v1` в `handlers.ts`. Они используют фактический контракт endpoint и предметные DTO из владеющей сущности. Общий отказ `MOCK_NOT_IMPLEMENTED` сохраняется: неописанный запрос не должен неожиданно обращаться к реальному складу.

Новые fixtures добавляются с соответствующим экраном. Для склада F05 даты построены относительно фиксированного `meta.server_time`, чтобы браузерные часы не влияли на статусы; UUID и ссылки согласованы, количества остаются decimal-строками. Для списков отдельно предусмотрены пустой раздел и поиск без результатов; такие ответы не применяются к healthcheck, у которого нет списка.

Сценарий фиксируется в начале запроса, до задержки. Переключение не меняет уже выполняющуюся попытку. Панель инвалидирует `Healthcheck`, `Inventory`, `Batches` и `ProductCategories`: обновляются активные подписки без удаления RTK Query cache.

Моковые ответы не подтверждают готовность backend. Настоящая интеграция проверяется с отключённым `VITE_ENABLE_MOCKS`; скрытого перехода с ошибок реального API на fixtures нет.

## Поставки (F06)

`supply-handlers.ts`/`supply-fixtures.ts` реализуют опубликованный GET списка: буквальный номер, created_at `[from,to)`, created_at DESC/id DESC, pagination и total. Даты создания/приёмки различаются; есть обе границы периода, одинаковые created_at и наносекунды. `supplies-empty` — пустая история, `supplies-received` — изменённый synthetic список после тестовой приёмки. Последний сценарий не выполняет POST и не проверяет реальную приёмку. Панель также инвалидирует `Supplies`, сохраняя текущий успешный cache. Подробности — `frontend/docs/supplies.md`.

## Auth (F03)

`auth-handlers.ts` использует плановые auth-поля README через документированный draft-адаптер; текущий OpenAPI уже публикует auth, однако адаптер и реальная интеграция ещё требуют отдельной проверки. Login выдаёт mock-cookie, me загружает user/settings/CSRF, logout проверяет mock-CSRF и удаляет mock-cookie. Fixtures не содержат production credentials. Панель переключает «Сценарий API», включая неверный пароль, отключённого пользователя и истечение. Подробности и ограничения — `frontend/docs/auth-session.md`.

## Роли и навигация (F04)

Сценарии role-admin, role-storekeeper и role-order-operator выбирают роль при login/me. Fixture success — manager; mock-only cookie flagman_mock_role сохраняет роль выданной тестовой сессии после reload и удаляется при logout. Переключение сценария не меняет уже принятую Redux-сессию до следующего me/login.

DEV-only /dev/access внутри оболочки использует healthcheck для проверки серверного 403 и тестового несекретного черновика. Forbidden показывает общий экран без деталей сервера, не очищая сессию; возврат к success восстанавливает черновик. Предметные запросы склада добавлены в F05. См. frontend/docs/navigation.md.

## Склад (F05)

`inventory-handlers.ts` и `inventory-fixtures.ts` реализуют опубликованные схемы, буквальный поиск/категорию, пересекающиеся серверные counts, сортировку null-last, пагинацию списка и партий. Fixture содержит годные/истекающие/просроченные/израсходованные партии, продукт только с просрочкой, нулевой остаток, неактивный продукт, точный большой агрегат и несколько строк одной поставки. Режим без status исключает depleted; `all` включает историю.

Сценарии `inventory-empty`, `batches-empty`, `batches-error`, `batches-slow`, `categories-error` независимы. Общие slow/network-error/forbidden/server-error/invalid-schema продолжают проверять транспорт. Сценарий фиксируется перед задержкой. Реальные API не используются в MSW-режиме; неизвестные маршруты сохраняют 501.

## Приёмка (F08)

reception-handlers.ts/reception-fixtures.ts добавляют GET /products и explicit POST /supplies. Панель содержит products-empty/error/slow и reception-slow/network/lost-response/validation/conflict/in-progress. Потеря ответа происходит после фиксации синтетического результата; повтор ключа/тела возвращает тот же SupplyDetail. Mock-only tab-local storage сохраняет результаты при reload. История, детали и склад читают синтетические принятые партии. Эти сценарии не доказывают настоящую серверную идемпотентность или CSRF/RBAC. Неизвестные endpoints остаются 501, production использует real API. Подробности — frontend/docs/supplies.md.

F16: административный каталог явно читает all/true/false с серверными фильтрами и sort=name. Категории используют опубликованные GET/POST/PATCH и отдельное синтетическое sessionStorage-состояние. Сценарии categories-empty, category-save-slow, category-revision-conflict, category-lost-response проверяют UI, а не реальный backend. Неизвестный исход записи не вызывает автоматического повторения.
