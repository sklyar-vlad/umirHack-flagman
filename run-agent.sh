#!/usr/bin/env bash
set -euo pipefail
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$project_dir"
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
for tool in git jq bd codex ralph python3 npm; do
  command -v "$tool" >/dev/null || { echo "Missing command: $tool" >&2; exit 1; }
done
branch=$(git symbolic-ref --quiet --short HEAD)
case "$branch" in agent/*) ;; *) echo 'Use an isolated agent/* branch' >&2; exit 1 ;; esac
git var GIT_AUTHOR_IDENT >/dev/null
git var GIT_COMMITTER_IDENT >/dev/null
bd --sandbox info >/dev/null
mkdir -p .ralph
# macOS ships no flock: atomic directory lock. A stale lock must be reviewed manually.
if ! mkdir .ralph/agent.lock 2>/dev/null; then
  echo 'Agent already running (or stale .ralph/agent.lock)' >&2
  exit 1
fi
printf '%s\n' "$$" > .ralph/agent.pid
cleanup() { rm -f .ralph/agent.pid; rmdir .ralph/agent.lock 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source "$HOME/.local/share/ralph-codex/secrets/yougile-project.env"
token_file="${YOUGILE_TOKEN_FILE:-$HOME/.local/share/ralph-codex/secrets/yougile-token}"
[[ -r "$token_file" ]] || { echo 'YouGile token file not readable' >&2; exit 1; }
export YOUGILE_TOKEN
YOUGILE_TOKEN=$(<"$token_file")
export BEADS_DOLT_AUTO_PUSH=false
export FRONTEND_DIR="${FRONTEND_DIR:-frontend}"
queue_status=$(./scripts/yougile-queue.sh prepare)
printf '%s\n' "$queue_status"
if jq -e '.status == "drained"' >/dev/null <<<"$queue_status"; then exit 0; fi
if [[ $# -eq 0 ]]; then set -- --unattended --no-sandbox --max-iterations 20; fi
ralph --tool codex "$@" &
ralph_pid=$!
trap 'kill -TERM "$ralph_pid" 2>/dev/null || true; wait "$ralph_pid" || true; exit 143' TERM INT
wait "$ralph_pid"
