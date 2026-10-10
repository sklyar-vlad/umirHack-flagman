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

Если работает один процесс, можно оставаться в основном каталоге проекта
и использовать отдельную рабочую ветку:

```bash
git switch -c agent/backend
```

Имя каталога менять не нужно. Ветка защищает `dev`/`main` от прямых коммитов;
launcher принимает ветки `agent/*`. Если выбранная ветка уже существует,
используйте `git switch agent/backend` вместо её повторного создания.

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
git commit -m "chore(git): exclude local workspace metadata"
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
    if [[ -f "$PROJECT_DIR/.ralph/paused.json" ]]; then
        log_error "Workflow paused. Start it again after the quota recovers."
        return 1
    fi
    if [[ "$tool" != codex ]]; then
        log_error "This configuration supports the Codex executor only."
        return 1
    fi
    _AI_STDIN=0
    _AI_CMD=(codex exec --approve-for-me --color never
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
    _bd dolt commit -m "Workflow iteration sync"
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
for tool in git jq bd codex ralph uvx flock python3; do
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
# Set budgets before Ralph initializes its zero-valued defaults.
export RALPH_MAX_RUN_TOKENS="${RALPH_MAX_RUN_TOKENS:-150000}"
export RALPH_MAX_RUN_SECONDS="${RALPH_MAX_RUN_SECONDS:-14400}"
export MAX_CONSECUTIVE_FAILURES="${MAX_CONSECUTIVE_FAILURES:-3}"
export FLAGMAN_TEST_DATABASE_URL="${FLAGMAN_TEST_DATABASE_URL:-postgres://postgres@127.0.0.1:55438/flagman_agent_test?sslmode=disable}"
export MENU_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export INVENTORY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export SUPPLY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export BATCH_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
if [[ $# -eq 0 ]]; then
    set -- --unattended --no-sandbox --max-iterations 12
fi
exec python3 scripts/quota_guard.py --prepare -- ralph --tool codex "$@"
```

### Файл `scripts/quota_guard.py`

```python
#!/usr/bin/env python3
"""Pause the local workflow with a five-percent subscription quota reserve."""
from __future__ import annotations

import json
import math
import os
from pathlib import Path
import select
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
STATE = ROOT / ".ralph" / "quota.json"
PAUSED = ROOT / ".ralph" / "paused.json"
RESERVE = 5
PAUSE_METADATA = "workflow_pause"


class QuotaUnavailable(RuntimeError):
    pass


class TaskStateUnavailable(RuntimeError):
    pass


class QuotaClient:
    """Read usage through the supported app-server account API, without a turn."""
    def __init__(self):
        self.process = subprocess.Popen(
            ["codex", "app-server", "--stdio"], cwd=ROOT,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, start_new_session=True,
        )
        self.buffer = b""
        self.request_id = 0
        try:
            self.request("initialize", {"clientInfo": {
                "name": "local_quota_guard", "title": "Local quota guard", "version": "1.0.0"
            }})
            self.send({"method": "initialized", "params": {}})
        except Exception:
            self.close()
            raise

    def send(self, message):
        try:
            self.process.stdin.write((json.dumps(message) + "\n").encode())
            self.process.stdin.flush()
        except (BrokenPipeError, OSError) as exc:
            raise QuotaUnavailable("Quota connection closed") from exc

    def request(self, method, params=None):
        self.request_id += 1
        message = {"id": self.request_id, "method": method}
        if params is not None:
            message["params"] = params
        self.send(message)
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            if b"\n" not in self.buffer:
                readable, _, _ = select.select([self.process.stdout], [], [], max(0, deadline-time.monotonic()))
                if not readable:
                    break
                chunk = os.read(self.process.stdout.fileno(), 65536)
                if not chunk:
                    break
                self.buffer += chunk
                continue
            line, self.buffer = self.buffer.split(b"\n", 1)
            try:
                response = json.loads(line)
            except ValueError:
                continue
            if response.get("id") != self.request_id:
                continue
            if "error" in response:
                # Never print account responses, authentication or raw server errors.
                raise QuotaUnavailable("Quota API rejected the request")
            return response.get("result", {})
        raise QuotaUnavailable("Quota API did not respond within 25 seconds")

    def read(self):
        for attempt in range(3):
            try:
                return summarize(self.request("account/rateLimits/read"))
            except QuotaUnavailable:
                if attempt == 2:
                    raise
                time.sleep(1)

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        for stream in (self.process.stdin, self.process.stdout):
            if stream:
                stream.close()


def summarize(result):
    buckets = result.get("rateLimitsByLimitId") or {}
    bucket = buckets.get("codex") or result.get("rateLimits") or {}
    windows = []
    for name in ("primary", "secondary"):
        value = bucket.get(name)
        if not isinstance(value, dict):
            continue
        used, minutes, reset = value.get("usedPercent"), value.get("windowDurationMins"), value.get("resetsAt")
        if (isinstance(used, bool) or not isinstance(used, (int, float))
                or not math.isfinite(used) or not 0 <= used <= 100
                or not isinstance(minutes, int) or minutes <= 0
                or not isinstance(reset, int) or reset <= 0):
            raise QuotaUnavailable("Quota API returned an invalid window")
        windows.append({"name": name, "used_percent": used,
                        "remaining_percent": 100-used, "window_minutes": minutes, "resets_at": reset})
    if not any(window["window_minutes"] == 300 for window in windows):
        raise QuotaUnavailable("The five-hour quota window is unavailable")
    return {"checked_at": int(time.time()), "reserve_percent": RESERVE, "windows": windows}


def pause_reason(quota):
    for window in quota["windows"]:
        if window["remaining_percent"] <= RESERVE:
            return {"reason": "quota_reserve", "window": window}
    return None


def write_json(filename, value):
    filename.parent.mkdir(parents=True, exist_ok=True)
    temporary = filename.with_suffix(".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    os.replace(temporary, filename)


def descendants(parent):
    entries = {}
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            stat = (entry / "stat").read_text().rsplit(")", 1)[1].split()
            entries[int(entry.name)] = (int(stat[1]), stat[19])
        except (OSError, ValueError, IndexError):
            pass
    children, frontier = {}, {parent}
    while frontier:
        next_frontier = {pid for pid, (ppid, _) in entries.items() if ppid in frontier and pid not in children}
        for pid in next_frontier:
            children[pid] = entries[pid][1]
        frontier = next_frontier
    return children


def interrupt_executors(parent):
    # Interrupt only Codex executors owned by this workflow; other sessions continue.
    executors = []
    for pid, start_time in descendants(parent).items():
        try:
            entry = Path(f"/proc/{pid}")
            args = (entry / "cmdline").read_bytes().split(b"\0")
            current_start = (entry / "stat").read_text().rsplit(")", 1)[1].split()[19]
            if current_start == start_time and Path(os.fsdecode(args[0])).name == "codex" and b"exec" in args:
                os.kill(pid, signal.SIGINT)
                executors.append(pid)
        except (OSError, ValueError, IndexError):
            pass
    # Give Codex time to finish interruption and persist its session.
    deadline = time.monotonic() + 20
    while executors and time.monotonic() < deadline:
        executors = [pid for pid in executors if Path(f"/proc/{pid}").exists()]
        if executors:
            time.sleep(0.2)


def stop_workflow(process):
    interrupt_executors(process.pid)
    if process.poll() is None:
        process.terminate()  # Ralph's EXIT handler cleans its isolated executor groups.
        try:
            process.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()


def beads(*args):
    try:
        result = subprocess.run(["bd", "--sandbox", *args], cwd=ROOT,
                                capture_output=True, text=True, timeout=10, check=True)
        return result.stdout
    except (OSError, subprocess.SubprocessError) as exc:
        raise TaskStateUnavailable("Could not update local task state") from exc


def task_metadata(task):
    metadata = task.get("metadata") or {}
    if isinstance(metadata, str):
        metadata = json.loads(metadata)
    return dict(metadata)


def owned_pause(task):
    marker = task_metadata(task).get(PAUSE_METADATA)
    return (isinstance(marker, dict) and marker.get("owner") == "local_workflow"
            and marker.get("previous_status") == "in_progress")


def block_active_tasks(reason):
    """Block only work owned by this queue; leave waiting and closed tasks intact."""
    blocked = []
    for task in json.loads(beads("list", "--all", "--limit", "0", "--json")):
        if not task.get("external_ref", "").startswith("yougile:"):
            continue
        if task["status"] == "blocked" and owned_pause(task):
            blocked.append(task)
            continue
        if task["status"] != "in_progress":
            continue
        metadata = task_metadata(task)
        marker = {"owner": "local_workflow", "previous_status": "in_progress",
                  "reason": reason["reason"], "paused_at": reason["paused_at"]}
        if reason.get("window"):
            marker["window"] = reason["window"]
        metadata[PAUSE_METADATA] = marker
        note = "Пауза выполнения: " + reason["reason"] + "."
        if reason.get("window"):
            note += " Остаток квоты: " + str(reason["window"]["remaining_percent"]) + "%."
        beads("update", task["id"], "--if-status", "in_progress", "--status", "blocked",
              "--metadata", json.dumps(metadata), "--append-notes", note)
        blocked.append({**task, "status": "blocked", "metadata": metadata})
    return blocked


def resume_paused_tasks():
    """Restore the guard's own pauses after a successful fresh quota check."""
    for task in json.loads(beads("list", "--all", "--limit", "0", "--json")):
        if (task.get("external_ref", "").startswith("yougile:")
                and task["status"] == "blocked" and owned_pause(task)):
            beads("update", task["id"], "--if-status", "blocked", "--status", "in_progress",
                  "--unset-metadata", PAUSE_METADATA, "--append-notes",
                  "Работа возобновлена после проверки квоты.")


def checkpoint(reason):
    value = {"status": "paused", "paused_at": int(time.time()), **reason}
    # Persist the reason before touching Beads, even if task synchronization fails.
    write_json(PAUSED, value)
    for name, args in (("head", ["git", "rev-parse", "HEAD"]),
                       ("working_tree", ["git", "status", "--porcelain"])):
        try:
            result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=15)
            if result.returncode == 0:
                value[name] = result.stdout.strip()
        except (OSError, ValueError, subprocess.TimeoutExpired):
            pass
    try:
        value["tasks"] = block_active_tasks(value)
    except (TaskStateUnavailable, ValueError, TypeError) as exc:
        value["task_state_error"] = str(exc)
    write_json(PAUSED, value)
    print("Workflow paused; files and unfinished tasks are preserved. Use the resume launcher.", flush=True)


def main():
    args = sys.argv[1:]
    read_only = args == ["--status"]
    prepare = args[:1] == ["--prepare"]
    if prepare:
        args = args[1:]
    if args[:1] == ["--"]:
        args = args[1:]
    if not read_only and not args:
        raise SystemExit("Usage: quota_guard.py --status | -- COMMAND [ARGS...]")
    client, process = None, None
    requested = []
    def request_stop(signum, _frame):
        requested.append(signum)
    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)
    try:
        client = QuotaClient()
        quota = client.read()
        if read_only:
            print(json.dumps(quota, ensure_ascii=False, indent=2))
            return 0
        write_json(STATE, quota)
        reason = pause_reason(quota)
        if reason:
            checkpoint(reason)
            return 0
        resume_paused_tasks()
        PAUSED.unlink(missing_ok=True)
        if prepare:
            result = subprocess.run([str(ROOT / "scripts/yougile-queue.sh"), "prepare"],
                                    cwd=ROOT, capture_output=True, text=True, check=True, timeout=300)
            print(result.stdout.strip(), flush=True)
            if json.loads(result.stdout)["status"] == "drained":
                return 0
        process = subprocess.Popen(args, cwd=ROOT, start_new_session=True)
        print("Quota guard enabled: pause at <=5% remaining; check every 15 seconds.", flush=True)
        while process.poll() is None:
            if requested:
                stop_workflow(process)
                checkpoint({"reason": "manual_pause"})
                return 0
            quota = client.read()
            write_json(STATE, quota)
            reason = pause_reason(quota)
            if reason:
                # Publish the flag before interruption so retries cannot start another turn.
                write_json(PAUSED, {"status": "pausing", **reason})
                stop_workflow(process)
                checkpoint(reason)
                return 0
            for _ in range(30):
                if process.poll() is not None or requested:
                    break
                time.sleep(0.5)
        return process.returncode
    except (QuotaUnavailable, TaskStateUnavailable, OSError) as exc:
        if read_only:
            print("Quota status is temporarily unavailable; the running workflow is unchanged.", file=sys.stderr)
            return 1
        if process is not None and process.poll() is None:
            stop_workflow(process)
        reason = "task_state_unavailable" if isinstance(exc, TaskStateUnavailable) else "quota_unavailable"
        checkpoint({"reason": reason, "detail": str(exc) if isinstance(exc, (QuotaUnavailable, TaskStateUnavailable)) else "Local quota reader failed"})
        return 1 if isinstance(exc, TaskStateUnavailable) else 0
    finally:
        if client:
            client.close()


if __name__ == "__main__":
    sys.exit(main())
```

### Файл `scripts/workflow-control.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd -- "$project_dir"
unit="${WORKFLOW_UNIT:-flagman-backend-agent}"
case "${1:-status}" in
    start|resume)
        if systemctl --user is-active --quiet "$unit.service"; then
            printf 'Workflow is already running.\n'
            exit 0
        fi
        systemctl --user reset-failed "$unit.service" 2>/dev/null || true
        systemd-run --user --unit="$unit" --collect \
            --property="WorkingDirectory=$project_dir" \
            --property=RuntimeMaxSec=14400 \
            --property=KillMode=mixed \
            --property=TimeoutStopSec=120 \
            --property="StandardOutput=append:$project_dir/.ralph/agent-console.log" \
            --property=StandardError=inherit \
            --setenv="PATH=$HOME/.local/bin:$HOME/Apps/go/bin:/usr/local/bin:/usr/bin:/bin" \
            --setenv="GOPATH=${GOPATH:-$HOME/Apps/go}" \
            /bin/bash "$project_dir/run-agent.sh" --unattended --no-sandbox --max-iterations 20
        if command -v notify-send >/dev/null; then
            notify-send 'Backend workflow' 'Запуск принят. Остаток квоты будет проверен перед работой.' || true
        fi
        ;;
    pause)
        systemctl --user stop "$unit.service"
        ;;
    status)
        systemctl --user show "$unit.service" --property=ActiveState,SubState,MainPID
        if [[ -f .ralph/quota.json ]]; then jq . .ralph/quota.json; fi
        if [[ -f .ralph/paused.json ]]; then jq '{status,reason,window,detail,paused_at,task_state_error,tasks:[.tasks[]? | {id,status}]}' .ralph/paused.json; fi
        ;;
    *) printf 'Usage: %s start|resume|pause|status\n' "$0" >&2; exit 1 ;;
esac
```

### Файл `.githooks/commit-msg`

```python
#!/usr/bin/env python3
"""Enforce the repository's commit-message convention."""
import re
import sys
from pathlib import Path

message = Path(sys.argv[1]).read_text()
forbidden = r"(?i)\b(?:agents?|codex|claude|chatgpt|ralph|serena|ai|neural)\b|нейросет|агент|co-authored-by|generated by"
if re.search(forbidden, message):
    print("Use a neutral commit message without tool attribution.", file=sys.stderr)
    sys.exit(1)
```

## 10. Проверка перед автономным запуском

Сделайте launcher и оболочку моста исполняемыми:

```bash
chmod +x run-agent.sh scripts/yougile-queue.sh
chmod +x .githooks/commit-msg
git config --local core.hooksPath .githooks
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
git add -- AGENTS.md .ralphrc ralph.json run-agent.sh scripts/yougile-queue.sh scripts/yougile_queue.py scripts/quota_guard.py scripts/workflow-control.sh .githooks/commit-msg
# Также явно добавьте нужные файлы skill/hooks, созданные bd setup codex.
git commit -m "chore(workflow): configure backend task processing"
./run-agent.sh --once
```

После первой итерации проверьте код, тесты, Git author/committer, состояние
story и подзадач в YouGile. Если всё в порядке:

Commit hook проверяет весь текст сообщения и запрещает упоминания агентов,
нейросетей, AI-инструментов, соавторские и Generated by подписи.
Исходный код настройки и документация содержат названия используемых инструментов;
ограничение относится к сообщениям коммитов.

```bash
./run-agent.sh --unattended --no-sandbox --max-iterations 20
```

Для фоновой работы после закрытия терминала используйте systemd user service:

```bash
chmod +x scripts/workflow-control.sh
./scripts/workflow-control.sh resume
./scripts/workflow-control.sh status
```

`flock` не допускает одновременный второй запуск в том же worktree. Максимум
20 итераций задаётся CLI; defaults в .ralphrc — 12 итераций, 150000 оценочных
токенов, 14400 секунд и 3 последовательные неудачи. Лимит токенов Ralph
оценочный и не равен точному денежному ограничению API. Учитывайте тариф
и ограничения используемой учётной записи Codex.

### Пауза при остатке 5% и продолжение одним нажатием

Launcher запускает Ralph через `scripts/quota_guard.py`. Контроллер получает
актуальные значения подписки из официального `account/rateLimits/read`
локального `codex app-server`, без запроса к модели. Он проверяет остаток
перед запуском и раз в 15 секунд. При остатке 5% или меньше в пятичасовом
или недельном окне текущая сессия Codex прерывается, Ralph останавливается,
а `.ralph/paused.json` сохраняет причину, HEAD, изменённые файлы и задачи.
Завершение story не требуется для паузы. В YouGile она остаётся в In progress.
В Beads активная story и начатые подзадачи становятся blocked; причина
сохраняется в notes и metadata workflow_pause. Ожидающие подзадачи остаются
open, завершённые остаются closed. Незавершённый код сохраняется,
проверки повторяются после продолжения. При возобновлении только задачи
с собственным маркером паузы возвращаются в in_progress; другие блокеры
launcher не снимает.
При недоступности данных квоты цикл также останавливается с сохранением.

После восстановления квоты:

```bash
./scripts/workflow-control.sh resume
```

Контроллер заново читает квоту и не запускает модель, если резерв ещё исчерпан.
Для ручной паузы используйте `./scripts/workflow-control.sh pause`.
Для кнопки в меню приложений создайте desktop-файл, подставив абсолютный
путь к worktree вместо `/absolute/path/to/project`:

```ini
[Desktop Entry]
Type=Application
Name=Продолжить backend
Comment=Продолжить текущую story после проверки квоты
Exec=/bin/bash /absolute/path/to/project/scripts/workflow-control.sh resume
Icon=media-playback-start
Terminal=false
Categories=Development;
Actions=Pause;

[Desktop Action Pause]
Name=Поставить на паузу
Exec=/bin/bash /absolute/path/to/project/scripts/workflow-control.sh pause
```

Сохраните его как `~/.local/share/applications/backend-workflow.desktop`.
Закрытие терминала не останавливает systemd user service. Выход из сессии
может его остановить; автозапуск при загрузке здесь не включён.
Процент отдаёт сервер: между проверками текущий запрос и другие сессии
могут расходовать квоту, поэтому математически точный резерв не гарантирован.
Это порог остатка подписки, отдельный от ограничения длительности Ralph.

## 11. Где смотреть очередь и результат

- YouGile: проект → доска → `To do(backend)` — ожидающие stories.
- `In progress` — текущая story; её подзадачи показывают поэтапное выполнение.
- `Done` — stories, прошедшие проверку и имеющие коммит.
- Beads содержит локальное зеркало выбранной story и долговременные блокеры.

```bash
bd --sandbox list --all
bd --sandbox ready
bd --sandbox list --status blocked
bd --sandbox blocked
tail -f .ralph/agent-console.log
cat .ralph/runs/latest/run.json
./scripts/workflow-control.sh status
git log -5 --format='%h %an <%ae> | %s'
git log -1 --format=fuller
git log -1 --format=%B
# Из исходного каталога, если worktree находится на отдельной ветке:
git log agent/s08 --format='%h %s'
git ls-files .serena .beads .beads.gate.lock
```

Последняя команда должна ничего не вывести. После остановки изучите причину
паузы в `bd --sandbox list --status blocked`: команда `bd blocked` в выбранной
версии показывает блокирующие зависимости, а не все задачи с этим статусом.
После остановки изучите причину
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
| Codex сообщает конфликт --approve-for-me и --sandbox | Оставить только --approve-for-me: он уже задаёт workspace-write |
| Нет коммита story в git log | Проверить ветку worktree; в исходном каталоге посмотреть git log agent/s08 |
| Кнопка продолжения снова оставила паузу | Проверить .ralph/paused.json и квоту: пока остаётся <=5%, модель не запускается |

## 13. Первичные источники

- [Codex CLI: MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).
- [Codex: автоматическая проверка разрешений](https://learn.chatgpt.com/docs/sandboxing/auto-review).
- [Codex: неинтерактивный запуск](https://learn.chatgpt.com/docs/non-interactive-mode).
- [Codex App Server: чтение квоты](https://learn.chatgpt.com/docs/app-server).
- [Ralph, выбранная реализация](https://github.com/WomB0ComB0/ralph).
- [Beads: установка](https://github.com/gastownhall/beads/blob/main/docs/getting-started/installation.md).
- [Serena 1.7.0](https://github.com/oraios/serena/releases/tag/v1.7.0).
- [Serena: запуск MCP](https://oraios.github.io/serena/02-usage/020_running.html).

Совместимость .ralphrc и моста проверяется по закреплённому коду и реальным
CLI-командам. У YouGile используются опубликованные MCP-инструменты
get_project_tree, list_users, query_tasks, get_task_data, set_assigned и manage_tasks;
описания и схемы получает клиент через tools/list самого MCP-сервера.
