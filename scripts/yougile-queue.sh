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
exec "$HOME/.local/share/ralph-codex/bridge-venv/bin/python" scripts/yougile_queue.py "$@"
