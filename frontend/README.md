# Кладовая — frontend

React/TypeScript интерфейс ресторанного склада: поставки и партии, заказы, ручные списания, стоп-лист, рекомендации, отчёты и административные справочники. Количества, FEFO, доступность, прогнозы и история рассчитываются сервером. Проверка корзины не резервирует запас.

## Запуск и проверки

Требуются Node.js ≥22.22 и npm. Из корня репозитория:

```sh
cd frontend
npm ci
npm run dev
```

Vite проксирует `/api` на `http://api.flagman.localhost`. Для реального API запустите [локальную инфраструктуру](../infrastructure/README.md); frontend использует относительный `/api/v1` и cookie того же origin. `npm run preview` показывает сборку, но не заменяет nginx с API-прокси.

| Команда (из frontend) | Проверка |
|---|---|
| `npm run lint` | ESLint |
| `npm run typecheck` | TypeScript |
| `npm run test` | Все Vitest-тесты, без production credentials |
| `npm run build` | TypeScript и `dist/` |
| `npm run test:browser` | Все браузерные сценарии, явный development MSW |
| `npm run test:acceptance` | Выбранные операции, ошибки, роли, клавиатура, SSE и адаптация |
| `npm run test:production` | Изолированные PostgreSQL/backend/nginx/Traefik и настоящий API |

Обязательный общий gate из корня: `./scripts/frontend-checks.sh` (lint → TypeScript → Vitest → build). Для ограниченной памяти используйте `VITEST_MAX_WORKERS=2`; утверждения и таймауты тестов остаются прежними.

Перед браузерными проверками выполните `npx playwright install chromium` (в Linux CI — `--with-deps chromium`). Можно задать `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` для установленного Chromium. Development-тесты запускают Vite на `127.0.0.1:5182`; этот порт должен быть свободен. Проверяются ширины 360/390/1024/1440, нативные диалоги и клавиатура. Скриншоты находятся в игнорируемом `test-results/`.

[Frontend CI](../.github/workflows/frontend-ci.yaml) дополняет существующий backend CI S27: `npm ci`, полный gate, выбранные MSW-сценарии и отдельная production-проверка. Зависимости устанавливаются по `package-lock.json`; Actions закреплены по SHA. Backend CI и его Dockerfile сохранены.

## Маршруты и API

| Маршруты | Основные опубликованные API | Документация |
|---|---|---|
| `/login`, `/logout` | `/auth/login`, `/auth/me`, `/auth/logout` | [Сессия](docs/auth-session.md) |
| `/`, `/dashboard` | `/dashboard` | [Обзор](docs/dashboard.md) |
| `/inventory` | `/inventory`, `/products/{id}/batches`, `/product-categories` | [Склад](docs/inventory.md) |
| `/supplies`, `/supplies/new`, `/supplies/:id` | GET/POST `/supplies`, GET `/supplies/{id}`, `/products` | [Поставки](docs/supplies.md) |
| `/orders`, `/orders/new`, `/orders/:id` | `/orders`, `/orders/check`, `/order-attempts` | [Заказы](docs/orders.md) |
| `/stop-list` | `/stop-list` | [Стоп-лист](docs/stop-list.md) |
| `/recommendations/offers` | `/hot-offers`, `/dishes/{id}` | [Предложения](docs/hot-offers.md) |
| `/recommendations/procurement` | `/procurement-recommendations` | [Закупки](docs/procurement.md) |
| `/write-offs`, действие в партии склада | POST `/write-offs` | [Списания](docs/write-off.md) |
| `/stock-movements` (также `/movements`) | `/stock-movements` | История партий и движения |
| `/analytics/write-offs` (также `/analytics`) | `/analytics/write-offs` | Отчёт о списаниях |
| `/analytics/ingredient-shortages` | `/analytics/ingredient-shortages` | [Недостача](docs/ingredient-shortages.md) |
| `/analytics/stop-list-forecast` (также `/forecast`) | `/analytics/stop-list-forecast` | [Прогноз](docs/stop-list-forecast.md) |
| `/admin/products`, `/admin/products/new`, `/admin/products/:id/edit`, `/admin/categories` | `/products`, `/product-categories` | [Продукты](docs/products.md) |
| `/admin/dishes`, `/admin/dishes/new`, `/admin/dishes/:id/edit` (также `/admin/recipes`) | `/dishes`, `/dishes/preview` | [Рецептуры](docs/dishes.md) |
| `/admin/users`, `/admin/users/new`, `/admin/users/:id/edit` | `/users`, `/users/{id}/password`, `/roles` | [Пользователи](docs/users.md) |
| `/admin/settings` | GET/PATCH `/settings` | [Настройки](docs/settings.md) |
| `/admin/audit-log` (также `/admin/audit`) | `/audit-log` | [Журнал администратора](docs/audit-log.md) |

Права определяет единая [матрица навигации](docs/navigation.md): admin — все разделы; manager — операции и аналитика; storekeeper — склад/приёмка/списания/закупки; order_operator — склад/заказы/стоп-лист/предложения. Запрещённая прямая ссылка не монтирует бизнес-запросы. Grafana остаётся техническим инструментом S26, отдельного ресторанного экрана мониторинга нет.

Контракт: [backend/docs/openapi.yaml](../backend/docs/openapi.yaml), [backend/docs/api.md](../backend/docs/api.md). Адаптер auth уже использует опубликованные поля login/me; его прежнее имя `draft-auth-adapter` сохранено для совместимости. Транспорт отправляет `credentials: include`, CSRF для записи и безопасные структурированные ошибки. Пароли не попадают в Redux, кеш или черновики. Создание поставки/заказа/списания сохраняет неизменяемые UUID ключа и тело перед отправкой: неизвестный результат допускает только явный идентичный повтор. Нет автоматического повтора записи или оптимистического расхода склада.

Одна session-scoped подписка `/events` обновляет разрешённые активные RTK Query чтения, сохраняет введённые поля и запускает проверку всей корзины. Native reconnect передаёт `Last-Event-ID`; resync/восстановление соединения выполняют перечитывание; фокус и видимость при здоровом SSE не инициируют запросы, ограниченный видимый polling работает только при обрыве. [SSE, ограничения и восстановление](docs/live-updates.md). [Текущая политика UI](docs/ui-behavior.md): фоновые обновления сохраняют формы, единый русский календарь, один активный раздел меню, время без секундного отсчёта.

## Явные моки

```sh
VITE_ENABLE_MOCKS=true VITE_MOCK_SCENARIO=success npm run dev
```

MSW запускается только в development/test. Панель «Моковый режим» выбирает сценарии ошибок, медленной сети, конфликтов, потерянного ответа, SSE/replay/resync. По умолчанию и во всех production-сборках используются реальные запросы. Невозможность загрузить MSW показывает отдельную ошибку; реальный запрос не подменяется автоматически. Синтетические результаты доказывают поведение frontend, а не работу сервера или его FEFO/транзакций.

## Production-приёмка и демонстрация

[Повторяемая приёмка и ручная демонстрация](docs/acceptance.md) разделяют реальные Docker/API проверки и синтетическую матрицу ошибок. Production-harness требует локальный Docker и `goose` v3.27.1. Он собирает существующие Dockerfile, создаёт отдельный Compose project с временной PostgreSQL, применяет миграции и генерирует случайные пароли для seed-users вне checkout. Существующие сервисы и данные приложения не изменяются; временные ресурсы удаляются после завершения.

Проверки production подтверждают обычный HTTP same-origin nginx, cookie/CSRF, native SSE, reload вложенного SPA-маршрута, FEFO и идемпотентность реальных операций. Они не подтверждают HTTPS/Secure cookie, конфигурацию внешнего ingress конкретной установки, работу фонового auto-writeoff или все производственные настройки. Эти границы указаны отдельно от готового frontend.

## Структура и общие правила

FSD: `app → pages → widgets → features → entities → shared`; внешние импорты идут через публичный `index.ts`, shared не импортирует верхние слои. Стек: React, TypeScript, Redux Toolkit/RTK Query, React Router, React Hook Form, Zod, Vite, Vitest, Playwright и MSW. Локальные Inter, SVG, UI-kit и CSS tokens находятся в `src/shared`; компоненты доступны через [публичный API UI-kit](src/shared/ui/index.ts).

Decimal-строки остаются строками в JSON/Redux. `shared/lib/quantity` переводит их в BigInt тысячных для точных операций, не округляет ввод и различает null/0; штуки целые. `shared/lib/restaurant-time` использует `settings.timezone`, сохраняет исходные instants и отклоняет неоднозначные DST значения. Количества и время исторических документов не заменяются текущими каталогами. Конфликт revision требует явного чтения и подтверждения, dirty-редакторы не меняются при фоновом обновлении. Черновики доступны только своему владельцу; logout/смена пользователя очищают их.
