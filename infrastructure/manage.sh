#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
env_file=${FLAGMAN_ENV_FILE:-./infrastructure/.env}
[ -f "$env_file" ] || { echo 'Create infrastructure/.env from infrastructure/.env.example' >&2; exit 2; }
mode=$(sed -n 's/^[[:space:]]*DEPLOY_MODE[[:space:]]*=[[:space:]]*//p' "$env_file" | tail -n 1 | tr -d '\r')
case "$mode" in
 ''|local|development|'"local"'|"'local'") mode=local ;;
 vps|'"vps"'|"'vps'") mode=vps ;;
 *) echo 'DEPLOY_MODE must be local or vps' >&2; exit 2 ;;
esac
dc() { docker compose --env-file "$env_file" -f "$root/infrastructure/compose.$mode.yaml" "$@"; }
case "${1:-}" in
 up) dc up -d --build --wait --wait-timeout 180 ;;
 down) dc down ;;
 logs) dc logs -f ;;
 seed) dc run --build --rm seed ;;
 *) echo 'Use up, down, logs or seed' >&2; exit 2 ;;
esac
