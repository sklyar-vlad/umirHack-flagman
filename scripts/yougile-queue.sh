#!/usr/bin/env bash
set -euo pipefail
project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd -- "$project_dir"
token_file="${YOUGILE_TOKEN_FILE:-$HOME/.local/share/ralph-codex/secrets/yougile-token}"
if [[ -f "$token_file" ]]; then
    export YOUGILE_TOKEN
    YOUGILE_TOKEN=$(<"$token_file")
fi
: "${YOUGILE_TOKEN:?YouGile token is required outside the repository}"
exec uvx --offline --from git+https://github.com/oraios/serena@949a27ef1e5fda1a6e7b561e777bcece345c6ffd \
    python scripts/yougile_queue.py "$@"
