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
        if [[ -f .ralph/paused.json ]]; then jq '{status,reason,window,detail,paused_at}' .ralph/paused.json; fi
        ;;
    *) printf 'Usage: %s start|resume|pause|status\n' "$0" >&2; exit 1 ;;
esac
