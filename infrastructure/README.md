# Локальный запуск

Команды выполняются из корня репозитория. Если `infrastructure/.env` отсутствует,
скопируйте `infrastructure/.env.example` в `infrastructure/.env`.

```sh
task up
```

`task up` запускает PostgreSQL, применяет миграции и поднимает приложение.
Для добавления начальных справочников и демоданных отдельно выполните `task seed`.

- Frontend: http://flagman.localhost
- Healthcheck backend: http://api.flagman.localhost/api/v1/healthcheck
- Панель Traefik: http://traefik.flagman.localhost/dashboard/

Frontend также проксирует `/api/` в backend, поэтому healthcheck доступен
по адресу http://flagman.localhost/api/v1/healthcheck.

Backend стартует после проверки готовности PostgreSQL. HTTP-порт backend
задаёт `PORT`; конфигурация Compose рассчитана на `8080`.
При изменении реквизитов БД обновите `POSTGRES_URL` и `GOOSE_DBSTRING`.
Backend подключается к `db:5432` внутри Docker, а Goose и команда
`seed-users` — к `localhost:5432` с хоста.

Для подключения к PostgreSQL из VS Code используйте host `127.0.0.1`, port
`5432`, database из `POSTGRES_DB`, user из `POSTGRES_USER` и пароль из
`POSTGRES_PASS` в `infrastructure/.env`. SSL для локального подключения
отключён. Порт БД опубликован на хосте;
backend внутри Docker продолжает подключаться к `db:5432`.

```sh
docker compose -f infrastructure/compose.yaml logs -f
docker compose -f infrastructure/compose.yaml down
```

Данные PostgreSQL сохраняются в Docker volume после остановки.

## Seed и сброс demo-окружения

### Обычный seed

Перед первым запуском заполните в локальном `infrastructure/.env`
`DEMO_ADMIN_PASSWORD`, `DEMO_MANAGER_PASSWORD`, `DEMO_STOREKEEPER_PASSWORD`
и `DEMO_ORDER_OPERATOR_PASSWORD`. Каждый пароль должен содержать
12–128 символов. Значения паролей не добавляются в репозиторий;
в базе сохраняются хеши Argon2id со случайной солью.

```sh
task seed
```

Команда запускает PostgreSQL, применяет миграции из
`backend/database/migrations`, создаёт отсутствующих демопользователей,
затем выполняет `001_catalog.sql` и `002_inventory.sql` из
`backend/database/seeds`.

Seed добавляет отсутствующие демоданные. Пользовательские поставки,
заказы и движения склада не удаляются. Существующие справочники,
рецептуры, настройки, остатки партий и пароли пользователей
не перезаписываются. Сброс базы в обычный seed не входит.

При конфликте уникальных данных команда завершается ошибкой, сохраняя
существующие записи. Такой конфликт не является поводом запускать сброс
базы с пользовательскими данными.

Даты демопартий строятся относительно времени первого создания фикстуры.
Повторный seed сохраняет эти даты. Чтобы снова показать демосценарий
со свежими сроками, выполните явный сброс выделенной demo-базы.

### Подготовка demo-базы

Для demo используется отдельная база `postgres_demo` в существующем
PostgreSQL. Имя `postgres_demo` зарезервировано для демонстрационных
данных, которые разрешено полностью удалять при сбросе.

Новый файл конфигурации не нужен. В существующем `infrastructure/.env`
оставьте по одному определению каждого параметра:

```dotenv
ENV=demo
POSTGRES_DB=postgres_demo
```

В `POSTGRES_URL` и `GOOSE_DBSTRING` также укажите базу `postgres_demo`.
Пользователя, пароль и адрес подключения сохраните. Backend использует
адрес `db:5432`, а Goose и `seed-users` — `localhost:5432`.

Если используется существующий Docker volume, изменение `POSTGRES_DB`
не создаёт новую базу автоматически. Если `postgres_demo` ещё нет,
подключитесь к базе `postgres` и один раз выполните:

```sql
CREATE DATABASE postgres_demo;
```

При создании нового пустого PostgreSQL volume образ создаёт базу
из `POSTGRES_DB` при первичной инициализации. Другие существующие базы
не нужно удалять для подготовки demo.

Затем выполните:

```sh
task seed
task up
```

### Явный полный сброс demo

PostgreSQL должен быть запущен. Убедитесь, что в `.env` установлены
`ENV=demo` и `POSTGRES_DB=postgres_demo`, а обе строки подключения
указывают на `postgres_demo`.

Сброс удаляет все данные приложения в demo-базе: пользователей,
справочники, рецептуры, настройки, поставки, партии, заказы,
движения склада, отклонённые попытки заказов, сессии, ключи
идемпотентности, аудит, outbox и историю миграций Goose.
Введённые вручную данные этой demo-базы также удаляются.

```sh
task demo-reset CONFIRM=postgres-demo
task seed
task up
```

`demo-reset` проверяет режим, имя базы и явное подтверждение удаления,
затем останавливает backend и frontend. Скрипт
`backend/database/seeds/reset_demo.sql` дополнительно проверяет
`current_database() = 'postgres_demo'` перед очисткой схемы `public`.
Очистка и восстановление пустой схемы выполняются в одной транзакции.

`task seed` после сброса заново применяет миграции и создаёт демоданные
с датами относительно текущего времени. Новые хеши паролей строятся
из актуальной demo-конфигурации. `task up` запускает приложение.

Docker volume сохраняется. Остальные базы PostgreSQL не очищаются.
Для demo-сброса не используйте `docker compose down -v` или общий
`task rollback`: эти команды не ограничены выделенной demo-базой
проверками `demo-reset`.

### Проверка поведения

`task demo-reset` без `CONFIRM=postgres-demo` должен завершаться ошибкой
до остановки сервисов и очистки базы. Такой же отказ ожидается при
другом `ENV` или `POSTGRES_DB`.

Повторный `task seed` должен сохранять ранее созданные пользовательские
поставки и заказы, остатки существующих партий и хеши паролей.
Проверку выполняйте на базе, где эти записи уже есть.

Полную последовательность `demo-reset → seed → up` проверяйте только
на `postgres_demo`, все данные которой разрешено удалить.


## SSE

`/api/v1/events` в nginx использует HTTP/1.1, `proxy_buffering off`, отключённые
cache/gzip и часовые idle timeouts. Остальные API сохраняют обычную конфигурацию.
Traefik перед обоими сервисами использует немедленный flush и `writeTimeout=0s`
для полного ответа; buffering middleware к маршруту не подключается.
Go сохраняет обычный WriteTimeout для REST, SSE обновляет отдельный пятисекундный
дедлайн перед каждой записью/flush, включая heartbeat после длительного idle.
Не добавляйте response buffering или compression middleware к SSE.

Для проверки после входа сохраните cookie jar вне репозитория и выполните:

```sh
curl -N --cookie /tmp/flagman-cookies.txt http://flagman.localhost/api/v1/events
curl -N --cookie /tmp/flagman-cookies.txt -H 'Last-Event-ID: 123' http://flagman.localhost/api/v1/events
```

`Content-Type: text/event-stream`, `Cache-Control: no-cache`; первый комментарий
приходит сразу, heartbeat — раз в 20 секунд. Клиент обновляет доступные темы
при `inventory.time_changed`, а при `resync.required` перечитывает все открытые
разделы. Истёкшая или отозванная сессия закрывает соединение.

Проверка через отдельные nginx/Traefik test-контейнеры и настоящий `curl -N`:

```sh
FLAGMAN_TEST_SSE_PROXIES=1 go -C backend test -count=1 -run TestCurlUnbufferedThroughNginxAndTraefik ./internal/handler/events
```

Нужны `FLAGMAN_TEST_DATABASE_URL` с выделенной локальной test-базой, curl,
Docker и локальные образы `flagman-frontend:latest` (либо
`FLAGMAN_TEST_NGINX_IMAGE`) и `traefik:v3.7`. Тест не запускает Compose и не
обращается к БД приложения: использует временную test-схему, loopback-порты
и отдельные read-only контейнеры, которые удаляются после проверки.

Параметры прокси: [nginx proxy buffering](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_buffering),
[Traefik timeouts](https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/) и
[Traefik flushInterval](https://doc.traefik.io/traefik/reference/routing-configuration/http/load-balancing/service/).
