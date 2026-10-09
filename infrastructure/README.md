# Локальный запуск

Команды выполняются из корня репозитория. Если `infrastructure/.env` отсутствует,
скопируйте `infrastructure/.env.example` в `infrastructure/.env`.

```sh
docker compose -f infrastructure/compose.yaml up -d --build
```

- Frontend: http://flagman.localhost
- Healthcheck backend: http://api.flagman.localhost/api/v1/healthcheck
- Панель Traefik: http://traefik.flagman.localhost/dashboard/

Frontend также проксирует `/api/` в backend, поэтому healthcheck доступен
по адресу http://flagman.localhost/api/v1/healthcheck.

Backend стартует после проверки готовности PostgreSQL. HTTP-порт backend
задаёт `PORT`; конфигурация Compose рассчитана на `8080`.
При изменении реквизитов БД обновите и `POSTGRES_URL`.

```sh
docker compose -f infrastructure/compose.yaml logs -f
docker compose -f infrastructure/compose.yaml down
```

Данные PostgreSQL сохраняются в Docker volume после остановки.
