#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
export PATH="/opt/homebrew/bin:$HOME/.local/bin:$PATH"
mkdir -p .ralph
pid_file=.ralph/agent.pid
live_pid() {
  [[ -f "$pid_file" ]] || return 1
  pid=$(cat "$pid_file")
  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null || return 1
  ps -p "$pid" -o command= | grep -F "$root/run-agent.sh" >/dev/null
}
case "${1:-status}" in
  start)
    if live_pid || [[ -d .ralph/agent.lock ]]; then echo 'Already running, or lock needs inspection'; exit 1; fi
    logfile="$root/.ralph/background-$(date +%Y%m%d-%H%M%S).log"
    printf '%s\n' "$logfile" > .ralph/current-log
    python3 - "$root" "$logfile" <<'PY'
import os, subprocess, sys
from pathlib import Path
root, logfile = sys.argv[1:]
os.umask(0o077)
with open(logfile, 'ab', buffering=0) as log:
    process = subprocess.Popen(
        ['/usr/bin/caffeinate', '-i', '/opt/homebrew/bin/bash', root + '/run-agent.sh',
         '--unattended', '--no-sandbox', '--max-iterations', '20'],
        cwd=root, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT,
        start_new_session=True, close_fds=True,
    )
Path(root, '.ralph/caffeinate.pid').write_text(str(process.pid) + '\n')
PY
    echo "Started; log: $logfile"
    ;;
  status)
    if live_pid; then echo "Running: PID $pid"; else echo 'Stopped'; fi
    [[ ! -f .ralph/current-log ]] || cat .ralph/current-log
    bd --sandbox list --status in_progress --json
    bd --sandbox list --status blocked --json
    ;;
  logs)
    if [[ -f .ralph/state/last-run-id ]]; then
      run_log=".ralph/runs/$(cat .ralph/state/last-run-id)/ralph.log"
    else run_log="$(cat .ralph/current-log)"; fi
    tail -n 80 -f "$run_log"
    ;;
  stop)
    if live_pid; then kill -TERM "$pid"; echo "Stop requested for PID $pid"; else echo 'No matching agent process'; fi
    ;;
  *) echo 'Usage: agent-control.sh start|status|logs|stop' >&2; exit 1 ;;
esac
