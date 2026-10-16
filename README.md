# Кладовая

Поправки после защиты: [аналитика движения продуктов, управление предложениями и внешние поставки](docs/product-amendments.md).

Ресторанный склад: партии и сроки годности, заказы со списанием FEFO, стоп-лист, закупки, горячие предложения, аналитика и SSE. Backend: Go/Gin/PostgreSQL. Frontend: React/TypeScript/RTK Query.

## Запуск

На машине нужны только Docker с Compose и Task. Из корня проекта:

```sh
[ -f infrastructure/.env ] || cp infrastructure/.env.example infrastructure/.env
# При необходимости отредактировать infrastructure/.env.
task up
task seed
```

`DEPLOY_MODE=local` выбирает `compose.local.yaml`. Для VPS поставить `DEPLOY_MODE=vps`, указать реальный `FLAGMAN_DOMAIN`, `ACME_EMAIL` и собственные пароли/токены в том же `.env`. Домен должен вести на сервер, порты 80/443 — быть доступны для HTTPS.

Для существующего тома сохраните прежние `POSTGRES_DB`, `POSTGRES_USER` и `POSTGRES_PASS`; `POSTGRES_URL` должен соответствовать им. Эти переменные создают базу и пользователя только при инициализации пустого тома. Копирование новых значений из примера не меняет учётные данные сохранённой БД. Healthcheck проверяет подключение с указанными учётными данными.

Оба Compose-файла самостоятельные. `task up` собирает образы; контейнер `migrate` со встроенной Go-утилитой автоматически применяет только невыполненные SQL, затем стартует backend. Уже применённые версии не выполняются повторно. Установка Go/Goose и ручные команды миграций на сервере не нужны.

`task seed` запускает контейнер с демопользователями, каталогом, рецептами, настройками и партиями. Повторный seed сохраняет существующие пароли, остатки, историю и сроки партий.

| Сервис | Local | VPS |
|---|---|---|
| Приложение | http://flagman.localhost | https://ваш-домен |
| PostgreSQL | 127.0.0.1:5432 | Только внутри Docker |
| Grafana | 127.0.0.1:3000 | 127.0.0.1:3000 через SSH-туннель |
| Prometheus | 127.0.0.1:9090 | Только внутри Docker |
| Traefik dashboard | 127.0.0.1:8081 | 127.0.0.1:8081 через SSH-туннель |

Демопользователи: `admin@flagman.invalid`, `manager@flagman.invalid`, `storekeeper@flagman.invalid`, `operator@flagman.invalid`. Пароли — соответствующие `DEMO_*_PASSWORD` из `.env`.

```sh
task logs
task down
```

`task down` сохраняет тома БД и мониторинга. Существующий локальный том `flagman-postgres` продолжает использоваться. Данные local и VPS принадлежат разным окружениям.

## Разработка и проверки

```sh
bash scripts/backend-checks.sh all
bash scripts/integration-checks.sh backend
bash scripts/frontend-checks.sh
cd frontend
npm run test:acceptance
npm run test:production
```

Для разработки нужны Go и Node.js/npm; lint backend использует golangci-lint. Браузерные проверки требуют установленного Chromium. Integration/production проверки используют тот же `compose.local.yaml` с отдельным именем project, временными портами и БД; после завершения удаляют только свои ресурсы. CI запускает эти проверки.

Карта модулей и подготовка к защите: [DEFENSE_GUIDE.md](DEFENSE_GUIDE.md). Контракт API: [OpenAPI](backend/docs/openapi.yaml).
