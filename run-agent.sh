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
token_file="${YOUGILE_TOKEN_FILE:-$HOME/.local/share/ralph-codex/secrets/yougile-token}"
export YOUGILE_TOKEN
YOUGILE_TOKEN=$(<"$token_file")
export BEADS_DOLT_AUTO_PUSH=false
# Set budgets before Ralph initializes its zero-valued defaults.
export RALPH_MAX_RUN_TOKENS="${RALPH_MAX_RUN_TOKENS:-150000}"
export RALPH_MAX_RUN_SECONDS="${RALPH_MAX_RUN_SECONDS:-14400}"
export MAX_CONSECUTIVE_FAILURES="${MAX_CONSECUTIVE_FAILURES:-3}"
export FLAGMAN_TEST_DATABASE_URL="${FLAGMAN_TEST_DATABASE_URL:-postgres://postgres@127.0.0.1:55438/flagman_s08_test?sslmode=disable}"
export MENU_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export INVENTORY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export SUPPLY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
export BATCH_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
if [[ $# -eq 0 ]]; then
    set -- --unattended --no-sandbox --max-iterations 12
fi
exec python3 scripts/quota_guard.py --prepare -- ralph --tool codex "$@"
