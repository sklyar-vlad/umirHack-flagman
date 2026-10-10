# Автономный Go-agent: Arch Linux, Codex CLI, Ralph, Serena, Beads и YouGile

Настройка одного агента, который берёт backend stories по порядку, выполняет
их подзадачи, проверяет код, создаёт один коммит на story и обновляет YouGile.
При старте story переходит в `In progress`, после проверок и коммита — в `Done`.
Подзадачи отмечаются выполненными по мере готовности. Исполнитель story и
каждой подзадачи — выбранный пользователь YouGile.

Гайд не содержит токенов, личного Git identity или ID частного проекта.
Параметры своего проекта подставляются при настройке.

## 1. Проверенные версии и ограничения

Проверено на Arch Linux с Codex CLI 0.161.0, Beads 1.3.1, Bun 1.4.3,
Go 1.27.1, uv 0.12.21 и npm 12.2.0.

- Ralph: `WomB0ComB0/ralph`, commit `80ca69e2937e926247d95d48ac4f63b65e3b2c9b`.
- Serena: версия 1.7.0, commit `949a27ef1e5fda1a6e7b561e777bcece345c6ffd`.
- Go-модуль в примерах находится в `backend/`.
- Stories называются `S08. ...`, подзадачи — `S08-T01: ...`.
- Исходная очередь — `To do(backend)`, рабочая колонка — `In progress`,
  завершённые stories — `Done`.

Версии закреплены потому, что этой версии Ralph нужны адаптации к современному
Beads и явная настройка Codex. После обновления любого компонента повторите
пробный запуск на одной story.

## 2. Установка инструментов

Сначала обновите Arch и установите зависимости:

```bash
sudo pacman -Syu --needed git jq curl go gopls uv nodejs npm bc sqlite bubblewrap docker
mkdir -p "$HOME/.local/bin" "$HOME/.local/share"
export PATH="$HOME/.local/bin:$PATH"
```

Пакет Arch называется `uv`; он предоставляет `uvx`. Добавьте строку PATH
в `~/.zshrc` или `~/.bashrc`, если её там ещё нет:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Установите CLI в пользовательский каталог:

```bash
npm install --global --prefix "$HOME/.local" @openai/codex@0.161.0
npm install --global --prefix "$HOME/.local" --allow-scripts=@beads/bd @beads/bd@1.3.1
npm install --global --prefix "$HOME/.local" --allow-scripts=bun bun@1.4.3
codex login
```

Beads скачивает свой исполняемый файл в postinstall. Если npm установил пакет,
но заблокировал этот шаг:

```bash
npm rebuild --global --prefix "$HOME/.local" --allow-scripts=@beads/bd @beads/bd
```

Проверка:

```bash
codex --version
bd version
bun --version
uvx --version
go version
gopls version
```

Beads 1.3.1 содержит встроенный Dolt: отдельный сервер Dolt для этого сценария
не нужен. Docker понадобится только для выделенной тестовой PostgreSQL.

## 3. Установка Ralph

```bash
git clone https://github.com/WomB0ComB0/ralph "$HOME/.local/share/ralph-harness"
git -C "$HOME/.local/share/ralph-harness" checkout 80ca69e2937e926247d95d48ac4f63b65e3b2c9b
ln -s "$HOME/.local/share/ralph-harness/ralph.sh" "$HOME/.local/bin/ralph"
ralph --version
ralph --help
```

Эта настройка использует Codex, поэтому `ralph --setup` не нужен: он ставит
дополнительные CLI и зависимости для других провайдеров.

## 4. Serena MCP для Codex

Сначала загрузите закреплённую Serena и её зависимости:

```bash
uvx --from git+https://github.com/oraios/serena@949a27ef1e5fda1a6e7b561e777bcece345c6ffd serena --help
codex mcp add serena -- \
  uvx --from git+https://github.com/oraios/serena@949a27ef1e5fda1a6e7b561e777bcece345c6ffd \
  serena start-mcp-server --context codex --project-from-cwd --open-web-dashboard false
```

В уже созданной секции `serena` файла `~/.codex/config.toml` задайте таймауты.
Не создавайте вторую одноимённую секцию и сохраните остальные настройки:

```toml
[mcp_servers.serena]
command = "uvx"
args = ["--from", "git+https://github.com/oraios/serena@949a27ef1e5fda1a6e7b561e777bcece345c6ffd", "serena", "start-mcp-server", "--context", "codex", "--project-from-cwd", "--open-web-dashboard", "false"]
startup_timeout_sec = 120
tool_timeout_sec = 180
```

`--project-from-cwd` активирует репозиторий, из которого запускается Codex.
В Go-проекте укажите Go в локальном `.serena/project.yml`, если автоопределение
выбрало другой язык:

```yaml
project_name: my-backend
language_servers:
  - go
encoding: utf-8
ignore_all_files_in_gitignore: true
ignored_paths: [frontend, .ralph, .beads]
read_only: false
excluded_tools: []
initial_prompt: Follow AGENTS.md and work on the selected backend story.
```

## 5. YouGile MCP и хранение токена

Получите собственный MCP-токен YouGile. Храните его вне репозитория с правами
0600. Следующая команда читает токен скрыто, без значения в истории shell:

```bash
bash -c '
  umask 077
  mkdir -p "$HOME/.local/share/ralph-codex/secrets"
  chmod 700 "$HOME/.local/share/ralph-codex/secrets"
  read -r -s -p "YouGile MCP token: " yougile_secret
  printf "\n"
  printf "%s" "$yougile_secret" > "$HOME/.local/share/ralph-codex/secrets/yougile-token"
'
export YOUGILE_TOKEN="$(<"$HOME/.local/share/ralph-codex/secrets/yougile-token")"
codex mcp add yougile --url https://ru.yougile.com/data/ai/mcp --bearer-token-env-var YOUGILE_TOKEN
codex mcp list
```

Для интерактивного `codex` переменная должна быть доступна в shell, из которого
он запускается. Автономный launcher ниже читает файл сам. Конфигурация Codex
содержит имя переменной, без значения токена.

В Codex попросите получить `get_project_tree`, затем `list_users` для выбранного
проекта. Запишите технические UUID проекта, доски, трёх колонок и своего
пользователя. Названия и человекочитаемые номера карточек не заменяют UUID.

Создайте вне репозитория файл
`~/.local/share/ralph-codex/secrets/yougile-project.env`:

```bash
export YOUGILE_PROJECT_ID="PROJECT_UUID"
export YOUGILE_BOARD_ID="BOARD_UUID"
export YOUGILE_TODO_COLUMN_ID="TODO_BACKEND_COLUMN_UUID"
export YOUGILE_PROGRESS_COLUMN_ID="IN_PROGRESS_COLUMN_UUID"
export YOUGILE_DONE_COLUMN_ID="DONE_COLUMN_UUID"
export YOUGILE_USER_ID="YOUR_USER_UUID"
```

Сделайте права 0600. Значения должны соответствовать одному проекту и доске.
Автоматизация назначает пользователя в режиме `add`, сохраняя других исполнителей.

## 6. Git identity, рабочая ветка и локальное состояние

В исходном Go-репозитории проверьте или настройте свою учётную запись:

```bash
git config --local user.name "YOUR_GIT_NAME"
git config --local user.email "YOUR_VERIFIED_EMAIL"
git var GIT_AUTHOR_IDENT
git var GIT_COMMITTER_IDENT
git status --short
```

Если в основном каталоге работает другой агент, создайте отдельный worktree
от последнего готового коммита. Незакоммиченные изменения другого агента
в новый worktree не попадают:

```bash
git worktree add -b agent/backend ../my-project-agent HEAD
cd ../my-project-agent
```

Добавьте в `.gitignore`:

```gitignore
.serena/
.beads/
.beads.gate.lock
.ralph/
__pycache__/
*.pyc
```

Если эти каталоги уже отслеживаются, уберите их только из Git index:

```bash
git rm --cached -r --ignore-unmatch -- .serena .beads .beads.gate.lock
git add -- .gitignore
git commit -m "chore(git): ignore local agent state"
```

Локальные файлы останутся на диске. История не переписывается. Удаление из
удалённого репозитория появится только после отдельного осознанного push;
агент в описанном сценарии push не выполняет.

## 7. Beads и проектные инструкции

Сохраните существующий `AGENTS.md`, если он есть. Затем:

```bash
bd --sandbox init --quiet --non-interactive --skip-agents --skip-hooks --prefix umi
bd setup codex
ralph --init
```

В Git worktree Beads использует общую `.beads` исходного репозитория.
Не создавайте вторую очередь в `.ralph/beads`.

`bd setup codex` устанавливает skill, native hooks и раздел инструкций.
Сохраните эти разделы и допишите свои правила:

```markdown
## Autonomous backend workflow

- Work on one backend story per iteration, starting from S08 in ascending order.
- YouGile contains requirements; mirror the selected story and its subtasks in Beads.
- Use bd --sandbox for every operation. Never push or sync Beads remotely.
- Read the full story, dependencies and all subtasks. Claim the story and move it
  to In progress. Assign the configured YouGile user to the story and all subtasks.
- Use Serena for symbol navigation and preserve the existing project architecture.
- Implement the subtasks in order. Add meaningful tests and run gofmt.
- Run go -C backend test ./..., go -C backend vet ./..., go -C backend build ./....
- Test database queries against a dedicated local PostgreSQL, never production.
- After a subtask is implemented and verified, run
  ./scripts/yougile-queue.sh subtask <bd-id> to mark it complete in both systems.
- After all subtasks pass, create ONE conventional commit for the story, including
  its number in the subject, for example feat(menu): implement availability (S08).
- Stage explicit related files. Preserve other people's changes.
- Use the existing Git author and committer; never change identity or add AI
  attribution, Generated by text, or AI Co-authored-by trailers.
- Never amend, rebase, rewrite existing commits, push, or merge into dev/main/master.
- Never track .serena/, .beads/, or .beads.gate.lock.
- After verification and the commit, run
  ./scripts/yougile-queue.sh finish <bd-story-id>. It moves the story to Done
  and claims the next ready story. Continue coding in the next Ralph iteration.
- Make routine implementation decisions autonomously. If blocked, record the
  reason in Beads, leave the story unfinished and do not bypass failed checks.
- Keep Codex's OS sandbox enabled. Do not spawn extra agents, access production,
  run destructive migrations, modify unrelated files or expose secrets.
```

Проверьте созданные `.codex/config.toml`, `.codex/hooks.json` и
`.agents/skills/beads/`: native hooks должны быть включены. Эти инструкции
можно хранить в Git; секреты и локальные базы в них не размещаются.

## 8. Выделенная тестовая PostgreSQL

Используйте собственный локальный Docker. Запустите отдельный контейнер:

```bash
docker run --detach --name flagman-agent-test \
  --publish 127.0.0.1:55438:5432 \
  --env POSTGRES_HOST_AUTH_METHOD=trust \
  --env POSTGRES_DB=flagman_agent_test postgres:18-alpine
```

Этот пример предназначен для локальных тестов на компьютере с доверенными
пользователями. Порт доступен через loopback. Он не меняет базу приложения.
Убедитесь, что имена контейнера и порт не заняты. Если заняты, выберите другие
и измените тестовый URL в launcher.

Тесты создают отдельные схемы, применяют миграции и удаляют только свои схемы.
Новые интеграционные тесты должны читать `FLAGMAN_TEST_DATABASE_URL` или
соответствующий именной URL из окружения.

## 9. Конфигурация Ralph

Замените сгенерированный `ralph.json`:

```json
{
  "tool": "codex",
  "maxIterations": 12,
  "sandbox": false,
  "verbose": true,
  "projectType": "go",
  "commands": {
    "test": "go -C backend test ./...",
    "build": "go -C backend build ./...",
    "check": "go -C backend vet ./..."
  }
}
```

В закреплённой версии Ralph некоторые defaults перекрывают `ralph.json`, а
task engine использует старые параметры SQLite Beads. Поэтому файл `.ralphrc`
ниже явно выбирает Codex, подключает общую базу Beads и разрешает проверки
Go-модуля в `backend/`.

`--approve-for-me` передаёт запросы разрешений автоматическому reviewer Codex;
`-s workspace-write` сохраняет системный sandbox. Если reviewer отклонит
действие, агент фиксирует блокер и останавливает зависимую работу.
`--no-sandbox` в Ralph управляет его Docker-обёрткой, а не sandbox Codex.

Следующие файлы — готовая реализация моста. Сохраните каждый блок под указанным
именем; значение токена ни в один из них не добавляется.

### Файл `.ralphrc`

```bash
# Compatibility for WomB0ComB0/ralph 80ca69e and Beads 1.3.1.
# Ralph loads defaults before ralph.json; set runtime options here explicitly.
TOOL=codex
MAX_ITERATIONS=12
VERBOSE=true
SANDBOX_MODE=false

export RALPH_MAX_RUN_TOKENS="${RALPH_MAX_RUN_TOKENS:-150000}"
export RALPH_MAX_RUN_SECONDS="${RALPH_MAX_RUN_SECONDS:-14400}"
export MAX_CONSECUTIVE_FAILURES="${MAX_CONSECUTIVE_FAILURES:-3}"

# Keep the OS sandbox and route necessary approvals through Codex auto-review.
# Require Serena at startup instead of silently continuing without navigation.
_build_ai_cmd() {
    local tool="$1" model="$2"
    if [[ "$tool" != codex ]]; then
        log_error "This configuration supports the Codex executor only."
        return 1
    fi
    _AI_STDIN=0
    _AI_CMD=(codex exec --approve-for-me -s workspace-write --color never
        -c mcp_servers.serena.required=true -c mcp_servers.yougile.required=true)
    [[ -z "$model" ]] || _AI_CMD+=(--model "$model")
    return 0
}

# Both the agent and Ralph must use the project's .beads database.
# Upstream still initializes a separate SQLite database with --db-type.
_bd() {
    command "$BD_BIN" -C "$PROJECT_DIR" --sandbox "$@"
}

init_task_engine() {
    _bd info >/dev/null
}

_ralph_pending_tasks() {
    local status count total=0
    for status in open in_progress blocked deferred; do
        count=$(_bd count --status "$status" --quiet) || return 1
        if [[ ! "$count" =~ ^[0-9]+$ ]]; then
            log_error "Beads returned an invalid count for $status."
            return 1
        fi
        total=$((total + count))
    done
    printf '%s\n' "$total"
}

verify_beads_complete() {
    local pending
    pending=$(_ralph_pending_tasks) || return 1
    [[ "$pending" -eq 0 ]]
}

backlog_drained() {
    local closed
    verify_beads_complete || return 1
    closed=$(_bd count --status closed --quiet) || return 1
    [[ "$closed" =~ ^[0-9]+$ && "$closed" -gt 0 ]]
}

commit_task_state() {
    _bd dolt commit -m "${1:-Agent iteration sync}"
    if [[ "${LAST_VERIFY_OK:-false}" == true ]]; then
        "$PROJECT_DIR/scripts/yougile-queue.sh" prepare
    fi
}

# The Go module lives in backend. Permit only the three configured checks.
runtime_command_allowed() {
    case "${1:-}" in
        'go -C backend test ./...'|'go -C backend vet ./...'|'go -C backend build ./...') return 0 ;;
        *) return 1 ;;
    esac
}
```

### Файл `scripts/yougile-queue.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd -- "$project_dir"
source "$HOME/.local/share/ralph-codex/secrets/yougile-project.env"
token_file="${YOUGILE_TOKEN_FILE:-$HOME/.local/share/ralph-codex/secrets/yougile-token}"
if [[ -f "$token_file" ]]; then
    export YOUGILE_TOKEN
    YOUGILE_TOKEN=$(<"$token_file")
fi
: "${YOUGILE_TOKEN:?YouGile token is required outside the repository}"
exec uvx --offline --from git+https://github.com/oraios/serena@949a27ef1e5fda1a6e7b561e777bcece345c6ffd \
    python scripts/yougile_queue.py "$@"
```

### Файл `scripts/yougile_queue.py`

```python
"""Mirror one backend story at a time from YouGile into Beads.

Credentials are inherited through YOUGILE_TOKEN, never saved in project state.
All mutations are limited to the selected story and its existing subtasks.
"""
import asyncio
import html
import json
import os
import re
import subprocess
import sys
from pathlib import Path

from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client

PROJECT = os.environ["YOUGILE_PROJECT_ID"]
BOARD = os.environ["YOUGILE_BOARD_ID"]
TODO = os.environ["YOUGILE_TODO_COLUMN_ID"]
PROGRESS = os.environ["YOUGILE_PROGRESS_COLUMN_ID"]
DONE = os.environ["YOUGILE_DONE_COLUMN_ID"]
USER = os.environ["YOUGILE_USER_ID"]
ROOT = Path(__file__).resolve().parent.parent


def command(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def bd(*args):
    return command("bd", "--sandbox", *args)


def issues():
    return json.loads(bd("list", "--all", "--limit", "0", "--json"))


def plaintext(description):
    return html.unescape(re.sub(r"<[^>]+>", "", description.replace("</p>", "\n")))


async def call(session, tool, arguments):
    result = await session.call_tool(tool, arguments)
    if result.isError:
        raise RuntimeError(f"YouGile {tool} failed")
    for content in result.content:
        if content.type == "text":
            return json.loads(content.text)
    raise RuntimeError(f"YouGile {tool} returned no data")


def update(task_id, column=None, complete=None):
    return {"id": task_id, "title": None, "columnId": column,
            "description": None, "color": None, "complete": complete, "archived": None}


async def assign(session, task_id):
    await call(session, "set_assigned", {"id": task_id, "addMode": "add", "users": USER})


async def prepare(session):
    local = issues()
    pending = [i for i in local if i.get("external_ref", "").startswith("yougile:") and i["status"] != "closed"]
    if pending:
        stories = [i for i in pending if i["issue_type"] == "story"]
        print(json.dumps({"status": "active", "stories": [{"id": i["id"], "title": i["title"]} for i in stories]}, ensure_ascii=False))
        return
    code = """return {stories:tasks.filter(t=>t.projectId==='%s'&&t.boardId==='%s'&&!t.archived&&/^S\\d+\\./.test(t.title)).map(t=>({id:t.id,title:t.title,columnId:t.columnId,complete:t.complete,description:t.description,subtasks:t.getSubtasks().map(s=>({id:s.id,title:s.title,complete:s.complete,description:s.description}))}))};""" % (PROJECT, BOARD)
    data = await call(session, "query_tasks", {"jsCode": code, "cursor": ""})
    # query_tasks wraps non-array results in `result` on some server versions.
    data = data.get("result", data)
    remote = data["stories"]
    candidates = [t for t in remote if t["columnId"] == TODO and not t["complete"] and int(re.match(r"S(\d+)", t["title"])[1]) >= 8]
    candidates.sort(key=lambda t: int(re.match(r"S(\d+)", t["title"])[1]))
    if not candidates:
        print(json.dumps({"status": "drained"}))
        return
    story = candidates[0]
    description = plaintext(story["description"])
    dependencies = re.search(r"Зависимости:\s*([^\n]+)", description)
    if dependencies:
        for number in re.findall(r"S(\d+)", dependencies[1]):
            prerequisite = next((t for t in remote if re.match(r"S0*" + str(int(number)) + r"\.", t["title"])), None)
            if prerequisite is None or not (prerequisite["complete"] or prerequisite["columnId"] == DONE):
                raise RuntimeError(f"{story['title']} depends on unfinished S{number}")
    number = int(re.match(r"S(\d+)", story["title"])[1])
    issue_id = f"umi-s{number:02d}"
    external = "yougile:" + story["id"]
    metadata = json.dumps({"base_commit": command("git", "rev-parse", "HEAD"), "yougile_id": story["id"]})
    bd("create", story["title"], "--id", issue_id, "--type", "story", "--external-ref", external,
       "--description", description, "--metadata", metadata)
    for task in sorted(story["subtasks"], key=lambda t: t["title"]):
        sub_id = bd("create", task["title"], "--parent", issue_id, "--silent",
                    "--external-ref", "yougile:" + task["id"], "--description", plaintext(task["description"]) or task["title"])
        if task["complete"]:
            bd("close", sub_id, "--reason", "Already completed in YouGile")
        await assign(session, task["id"])
    await assign(session, story["id"])
    bd("update", issue_id, "--claim")
    await call(session, "manage_tasks", {"tasks": [update(story["id"], PROGRESS)]})
    print(json.dumps({"status": "claimed", "id": issue_id, "title": story["title"]}, ensure_ascii=False))


def verify():
    for action in ("test", "vet", "build"):
        subprocess.run(["go", "-C", "backend", action, "./..."], cwd=ROOT, check=True)


async def complete(session, issue_id, story=False):
    issue = json.loads(bd("show", issue_id, "--json"))[0]
    external = issue.get("external_ref", "")
    if not external.startswith("yougile:"):
        raise RuntimeError("Issue has no YouGile reference")
    remote_id = external.split(":", 1)[1]
    full = await call(session, "get_task_data", {"id": remote_id, "cursor": ""})
    task = full["taskFullInfo"]
    if task.get("projectId") != PROJECT or task.get("boardId") != BOARD:
        # Boardless subtasks report empty project/board fields. Validate their
        # actual parent relationship before mutating the subtask.
        authorized = False
        if not story:
            for parent_id in task.get("parentTaskIds", []):
                parent = (await call(session, "get_task_data", {"id": parent_id, "cursor": ""}))["taskFullInfo"]
                if parent.get("projectId") == PROJECT and parent.get("boardId") == BOARD and remote_id in parent.get("subtasks", []):
                    authorized = True
                    break
        if not authorized:
            raise RuntimeError("Task is outside the authorized board")
    verify()
    if story:
        if issue["issue_type"] != "story":
            raise RuntimeError("Expected a story")
        if command("git", "status", "--porcelain", "--untracked-files=normal"):
            raise RuntimeError("Commit all story changes before finishing")
        metadata = issue.get("metadata") or {}
        if isinstance(metadata, str):
            metadata = json.loads(metadata)
        if not metadata.get("base_commit") or command("git", "rev-parse", "HEAD") == command("git", "rev-parse", metadata["base_commit"]):
            raise RuntimeError("A new story commit is required")
        number = re.match(r"S\d+", issue["title"])[0]
        subjects = command("git", "log", metadata["base_commit"] + "..HEAD", "--format=%s")
        if not re.search(r"\b" + re.escape(number) + r"\b", subjects):
            raise RuntimeError(f"Include {number} in the story commit subject")
        children = json.loads(bd("list", "--parent", issue_id, "--all", "--limit", "0", "--json"))
        if any(child["status"] != "closed" for child in children):
            raise RuntimeError("Complete all subtasks first")
        await call(session, "manage_tasks", {"tasks": [update(remote_id, DONE, True)]})
    else:
        if issue["issue_type"] == "story":
            raise RuntimeError("Use finish for a story")
        await call(session, "manage_tasks", {"tasks": [update(remote_id, complete=True)]})
    bd("close", issue_id, "--reason", "Verified" + (" and committed" if story else ""))
    if story:
        await prepare(session)


async def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "prepare"
    if mode == "prepare":
        active = [i for i in issues() if i.get("external_ref", "").startswith("yougile:") and i["status"] != "closed"]
        if active:
            print(json.dumps({"status": "active", "issues": [{"id": i["id"], "title": i["title"]} for i in active]}, ensure_ascii=False))
            return
    token = os.environ["YOUGILE_TOKEN"].strip()
    async with streamablehttp_client("https://ru.yougile.com/data/ai/mcp", headers={"Authorization": "Bearer " + token}) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            if mode == "prepare":
                await prepare(session)
            elif mode in ("subtask", "finish") and len(sys.argv) == 3:
                await complete(session, sys.argv[2], story=mode == "finish")
            else:
                raise RuntimeError("Usage: yougile-queue.sh prepare | subtask <bd-id> | finish <bd-story-id>")


if __name__ == "__main__":
    try:
        asyncio.run(asyncio.wait_for(main(), 300))
    except Exception as error:
        # Do not print transport exceptions or request headers containing secrets.
        message = str(error).replace(os.environ.get("YOUGILE_TOKEN", ""), "[REDACTED]")
        if isinstance(error, BaseExceptionGroup):
            def causes(group):
                for item in group.exceptions:
                    if isinstance(item, BaseExceptionGroup):
                        yield from causes(item)
                    else:
                        yield type(item).__name__ + ": " + str(item)
            message = "; ".join(causes(error)).replace(os.environ.get("YOUGILE_TOKEN", ""), "[REDACTED]")
        message = re.sub(r"ygmcp_[A-Za-z0-9_-]+", "[REDACTED]", message)
        print(message, file=sys.stderr)
        sys.exit(1)
```

### Файл `run-agent.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd -- "$project_dir"
for tool in git jq bd codex ralph uvx flock; do
    command -v "$tool" >/dev/null || { printf 'Missing command: %s\n' "$tool" >&2; exit 1; }
done
branch=$(git symbolic-ref --quiet --short HEAD)
case "$branch" in
    agent/*) ;;
    *) printf 'Use an isolated agent branch.\n' >&2; exit 1 ;;
esac
git var GIT_AUTHOR_IDENT >/dev/null
git var GIT_COMMITTER_IDENT >/dev/null
bd --sandbox info >/dev/null
mkdir -p .ralph
exec 9>.ralph/agent.lock
flock -n 9 || { printf 'The agent is already running.\n' >&2; exit 1; }
source "$HOME/.local/share/ralph-codex/secrets/yougile-project.env"
token_file="${YOUGILE_TOKEN_FILE:-$HOME/.local/share/ralph-codex/secrets/yougile-token}"
export YOUGILE_TOKEN
YOUGILE_TOKEN=$(<"$token_file")
export BEADS_DOLT_AUTO_PUSH=false
export FLAGMAN_TEST_DATABASE_URL="${FLAGMAN_TEST_DATABASE_URL:-postgres://postgres@127.0.0.1:55438/flagman_agent_test?sslmode=disable}"
export MENU_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export INVENTORY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export SUPPLY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export BATCH_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
queue_status=$(./scripts/yougile-queue.sh prepare)
printf '%s\n' "$queue_status"
if jq -e '.status == "drained"' >/dev/null <<<"$queue_status"; then
    exit 0
fi
if [[ $# -eq 0 ]]; then
    set -- --unattended --no-sandbox --max-iterations 12
fi
exec ralph --tool codex "$@"
```

## 10. Проверка перед автономным запуском

Сделайте launcher и оболочку моста исполняемыми:

```bash
chmod +x run-agent.sh scripts/yougile-queue.sh
bash -n .ralphrc run-agent.sh scripts/yougile-queue.sh
bd --sandbox info
codex mcp list
```

Из worktree запустите `codex` и попросите выполнить проверку чтением:

```text
Use Serena to activate the current project and inspect its major Go modules.
Use YouGile get_project_tree and list_users to verify the configured project,
board, columns and user. Read the starting story and all subtasks.
Do not modify code or task statuses during this connectivity check.
```

Затем выполните проверки backend и выбранные интеграционные тесты на отдельной
БД. Убедитесь, что они реально запущены, а не пропущены из-за отсутствия URL.
Сохраните конфигурацию и инструкции отдельным setup-коммитом: при завершении
story мост требует чистое рабочее дерево.

```bash
git status --short
git add -- AGENTS.md .ralphrc ralph.json run-agent.sh scripts/yougile-queue.sh scripts/yougile_queue.py
# Также явно добавьте нужные файлы skill/hooks, созданные bd setup codex.
git commit -m "chore(agent): configure autonomous backend workflow"
./run-agent.sh --once
```

После первой итерации проверьте код, тесты, Git author/committer, состояние
story и подзадач в YouGile. Если всё в порядке:

```bash
./run-agent.sh --unattended --no-sandbox --max-iterations 20
```

Для работы после закрытия терминала:

```bash
mkdir -p .ralph
nohup ./run-agent.sh --unattended --no-sandbox --max-iterations 20 \
  > .ralph/agent-console.log 2>&1 < /dev/null &
printf '%s\n' "$!" > .ralph/agent.pid
```

`flock` не допускает одновременный второй запуск в том же worktree. Максимум
20 итераций задаётся CLI; defaults в .ralphrc — 12 итераций, 150000 оценочных
токенов, 14400 секунд и 3 последовательные неудачи. Лимит токенов Ralph
оценочный и не равен точному денежному ограничению API. Учитывайте тариф
и ограничения используемой учётной записи Codex.

## 11. Где смотреть очередь и результат

- YouGile: проект → доска → `To do(backend)` — ожидающие stories.
- `In progress` — текущая story; её подзадачи показывают поэтапное выполнение.
- `Done` — stories, прошедшие проверку и имеющие коммит.
- Beads содержит локальное зеркало выбранной story и долговременные блокеры.

```bash
bd --sandbox list --all
bd --sandbox ready
bd --sandbox blocked
tail -f .ralph/agent-console.log
cat .ralph/runs/latest/run.json
git log -5 --format='%h %an <%ae> | %s'
git log -1 --format=fuller
git log -1 --format=%B
git ls-files .serena .beads .beads.gate.lock
```

Последняя команда должна ничего не вывести. После остановки изучите причину
в журнале и Beads. Не закрывайте заблокированную story вручную для обхода
проверки. Исправив окружение или причину, повторно запустите launcher:
он продолжит с незавершённой локальной story.

## 12. Частые проблемы

| Симптом | Что проверить |
|---|---|
| YouGile возвращает 401 | Актуальность собственного MCP-токена и YOUGILE_TOKEN в окружении запуска |
| Serena долго стартует | startup_timeout_sec=120, установленный gopls, правильный Go-язык проекта |
| uvx --offline не находит Serena | Выполнить команду загрузки Serena из раздела 4 один раз с сетью |
| Ralph использует OpenCode или создаёт SQLite Beads | .ralphrc находится в корне запуска; версии соответствуют закреплённым |
| Go-проверки не запускаются | Go-модуль находится в backend; ralph.json и allowlist в .ralphrc совпадают |
| Интеграционные тесты пропущены | Правильные тестовые env-переменные и запущенная выделенная PostgreSQL |
| Docker недоступен | Локальный доступ пользователя к Docker; не подставлять production-БД вместо тестовой |
| Story не переходит в Done | Все подзадачи закрыты, дерево чистое, после начала story есть коммит с её номером |
| Нет готовой следующей story | Прочитать зависимости в YouGile; незавершённая зависимость останавливает выбор |
| Появился запрос системного разрешения | --approve-for-me в команде Codex; решение reviewer нельзя обходить отключением sandbox |

## 13. Первичные источники

- [Codex CLI: MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
- [Codex: автоматическая проверка разрешений](https://learn.chatgpt.com/docs/sandboxing/auto-review).
- [Codex: неинтерактивный запуск](https://learn.chatgpt.com/docs/non-interactive-mode).
- [Ralph, выбранная реализация](https://github.com/WomB0ComB0/ralph).
- [Beads: установка](https://github.com/gastownhall/beads/blob/main/docs/getting-started/installation.md).
- [Serena 1.7.0](https://github.com/oraios/serena/releases/tag/v1.7.0).
- [Serena: запуск MCP](https://oraios.github.io/serena/02-usage/020_running.html).

Совместимость .ralphrc и моста проверяется по закреплённому коду и реальным
CLI-командам. У YouGile используются опубликованные MCP-инструменты
get_project_tree, list_users, query_tasks, get_task_data, set_assigned и manage_tasks;
описания и схемы получает клиент через tools/list самого MCP-сервера.
