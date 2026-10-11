# Проверки backend

GitHub Actions запускает `.github/workflows/backend-ci.yaml` для pull request,
каждого push в `dev` и вручную. Права — только `contents: read`; checkout не
сохраняет Git credentials. Workflow не публикует образ и не выполняет deploy.

## Проверки исходников

Требуются Go из `go.mod`, Task 3.51.1 и golangci-lint 2.14.0.

```sh
go install github.com/go-task/task/v3/cmd/task@v3.51.1
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
task backend:check
```

Команда последовательно проверяет gofmt, существующий `backend/.golangci.yaml`,
модульные тесты (`go test -count=1 ./...`), `go vet ./...` и `go build ./...`.
Проверки не исправляют исходники. Старый `task lint` остаётся явной командой
форматирования/исправления для разработчика. По отдельности доступны
`backend:format-check`, `backend:lint`, `backend:test`, `backend:vet`, `backend:build`.

`backend:test` очищает все test database URL и флаги контейнерных интеграций
в дочернем процессе: unit-проверки не зависят от локальных credentials.
В них входят точные decimal/pcs вычисления, FEFO, доступность меню, закупки,
горячие предложения и моделирование прогноза. Пропуски PostgreSQL тестов здесь
ожидаемы; отдельный интеграционный этап обязан исполнить их с реальной БД.

При ограниченном доступе к домашнему каталогу можно задать writable cache:
`GOLANGCI_LINT_CACHE=/tmp/flagman-lint-cache task backend:check`.
Контрактные тесты проверяют отказ при неформатированном файле, остановку после
ошибки lint и очистку унаследованных интеграционных адресов:

```sh
python3 -m unittest discover -s scripts -p 'test_backend_ci.py' -v
```

## Транзакционные интеграции PostgreSQL

Дополнительно нужны локальный Docker daemon (Unix socket), Docker Compose и
Goose 3.27.1; удалённый Docker endpoint отклоняется до создания ресурсов.

```sh
go install github.com/pressly/goose/v3/cmd/goose@v3.27.1
task backend:integration
```

Команда поднимает отдельный `postgres:18-alpine`, новую `flagman_ci_test` на tmpfs,
уникальный Compose project и случайный **loopback** порт. Приложение, его `.env`,
его volume и БД не используются. CI-сеть — отдельный bridge: internal-сети Docker
не публикуют нужный для host-тестов порт. Публикация БД ограничена `127.0.0.1`.
Пароль создаётся для каждого запуска в приватном временном каталоге вне checkout,
передаётся через файл секрета, не выводится в stdout и удаляется при завершении.

Сначала Goose применяет реальные forward-миграции с version ledger, без dotenv.
Затем все шесть URL (`FLAGMAN`, `PRODUCT`, `SUPPLY`, `MENU`, `BATCH`, `INVENTORY`
`_TEST_DATABASE_URL`) указывают только на эту БД. `go test -json -count=1 -p 4`
выполняет полный suite: реальные rollback/commit-time ошибки, lock waits,
конкурентные заказы/списания, replay/idempotency, исторические снимки и SSE.
Тесты сохраняют существующую изоляцию собственными схемами.

Проверка результатов требует успешного исполнения ключевых transaction тестов
и отклоняет любой неожиданный skip. Ровно пять самостоятельных тестов
backup/monitoring/TLS/proxy допускают skip в этом этапе; они требуют дополнительных
операционных образов и проверялись при реализации соответствующих функций.
API smoke в CI выполняется отдельно. Ошибка миграции, компиляции, теста,
пропуск обязательного suite или ошибка cleanup дают ненулевой статус.

Exit/INT/TERM cleanup удаляет только текущий случайный Compose project и его
временные файлы. Проекты разработки и PostgreSQL volumes приложения не затрагиваются.
Для повторной проверки достаточно повторить команду: состояние предыдущего
запуска не переиспользуется. Требования к именованным test URL для ручного
`go test` сохраняются; CI-команда намеренно игнорирует унаследованные адреса.

## Docker и API smoke

```sh
task backend:smoke
```

Требования те же, что для интеграций. Smoke создаёт свою отдельную временную БД,
применяет Goose и собирает `infrastructure/Dockerfile.backend` из копии backend
во временном каталоге. Реальные dotenv, сертификаты и архивы туда не копируются.
Синтетические `.env`, `.env.*`, `.pem`, `.key`, `.dump` canary-файлы проверяют,
что правила `backend/.dockerignore` исключает секреты и из build stage. В `/app`
готового runtime-образа обязан остаться только исполняемый `server`.

Образ запускается только в standalone `infrastructure/compose.ci.yaml` с отдельным
Compose project, без Traefik/frontend, без приложения `.env` и его volumes.
`POSTGRES_URL_FILE` и PostgreSQL password передаются внешними файлами.
`ENV=development`, `PORT=8080`, `FRONTEND_ORIGIN=http://flagman-ci.invalid` заданы
явно; API публикуется только на случайном `127.0.0.1` порту. Это локальная HTTP
проверка, не замена HTTPS rollout по [HTTPS.md](../infrastructure/HTTPS.md).

Обычные seed-users и SQL seeds каталога/склада создают данные в CI-базе
с одноразовым паролем пользователей. Backend запускается с UID/GID владельца
mode600 файлов секретов и без capabilities.
Реальный HTTP-клиент без redirects/proxy проверяет liveness/readiness, отказ
без сессии, Origin/CORS, login/me/settings/inventory/menu, запрет admin API
менеджеру, CSRF и logout. После остановки **только CI** PostgreSQL readiness
обязана вернуть 503, а liveness — 200. Пароль не должен попасть в backend logs.
Cookies/CSRF и ответы login не печатаются, хранятся только в памяти клиента.

Cleanup удаляет оба уникальных image tags, контейнеры/сеть и временные секреты.
Buildx metadata также находится в приватном временном каталоге. Build cache Docker
может сохранять слои исходников, но synthetic secrets исключены из COPY, реальные
секреты не входят в context. Images не push-ятся; секреты GitHub/VPS не нужны.

CI запускает checks, затем integration и smoke. Настройку required check `Backend CI`
и первый hosted-run после push выполняет владелец репозитория. Локальный проход
этих же команд подтверждает исполняемую конфигурацию; remote deploy отсутствует.

Полный проверяемый сценарий перед защитой запускается отдельно командой
`task backend:demo`. Он использует тот же механизм владения одноразовой БД и
образом, создаёт бизнес-данные обычными API и дополнительно проверяет админку,
списания/отчёты, SSE/retention/reconnect, конкурентные заказы, restart и настоящие
Prometheus/Grafana. У monitoring свои временные секреты и tmpfs, cleanup включает
оба profile-сервиса. Этапы и точные проверяемые факты описаны в [DEMO.md](DEMO.md).
