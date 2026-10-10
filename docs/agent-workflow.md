# Автономная разработка backend

Рабочая ветка `agent/s08` находится в отдельном worktree
`/home/sklyarvlad/work/umirHack-flagman-agent-s08`. Очередь требований:
YouGile → `umirHack-flagman` → `dev` → `To do(backend)`.
Stories выбираются по номеру S, начиная с S08. Текущая story находится
в `In progress`, завершённая — в `Done`. Vladislav назначается исполнителем
story и каждой подзадачи; подзадачи отмечаются выполненными после проверки.

Из рабочего каталога:

```bash
bd --sandbox list                 # локальные задачи и состояния
bd --sandbox ready                # доступные задачи
bd --sandbox blocked              # блокеры
git log -5 --format='%h %an <%ae> | %s'
./run-agent.sh                    # автономный запуск, максимум 12 итераций
./run-agent.sh --once              # одна итерация
tail -f .ralph/agent-console.log   # журнал запущенного фонового цикла
```

Ralph использует Codex CLI с `--approve-for-me`, sandbox `workspace-write`
и обязательными Serena/YouGile MCP. `--no-sandbox` в Ralph отключает только
его дополнительный Docker-контейнер. Системный sandbox Codex остаётся включён.
Один цикл использует существующий Git identity, создаёт один conventional
commit на story и продолжает со следующей. Push, слияния в dev/main/master
и переписывание истории запрещены. Одновременный второй цикл блокирует flock.

Привязка к YouGile хранится в `external_ref` задач Beads. База Beads общая
для worktree и исходного репозитория; `bd` находит её автоматически.
`scripts/yougile-queue.sh prepare` импортирует, назначает и берёт следующую
story. `subtask <bd-id>` проверяет backend и закрывает подзадачу в обеих
системах. `finish <bd-story-id>` требует чистое дерево и новый коммит с
номером story в заголовке, закрывает story и берёт следующую.

Токен расположен вне репозитория, в
`~/.local/share/ralph-codex/secrets/yougile-token` с правами 0600.
Launcher передаёт его через `YOUGILE_TOKEN`; в командную строку и журналы
токен не попадает. `.serena/`, `.beads/`, `.beads.gate.lock` и состояние
`.ralph/` исключены через `.gitignore`.

Для интеграционных тестов используется отдельный контейнер `flagman-s08-test`
с PostgreSQL на `127.0.0.1:55438`, база `flagman_s08_test`. Приложение и база
другого агента не используются. Launcher экспортирует тестовые URL. Новые
тесты используют `FLAGMAN_TEST_DATABASE_URL` либо соответствующий именной URL.
Настройки можно переопределить через `FLAGMAN_TEST_DATABASE_URL`.

Лимиты по умолчанию: 12 итераций, 150000 оценочных токенов, 4 часа,
3 последовательных неудачи. Это ограничение запуска, а не точный бюджет API.
Блокер сохраняется в Beads, незавершённая story остаётся в `In progress`.
