#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
env_file=${FLAGMAN_ENV_FILE:-./infrastructure/.env}
case "$env_file" in /*) ;; *) env_file="$root/$env_file";; esac
[ -f "$env_file" ] || { echo 'Create infrastructure/.env from the local or VPS example' >&2; exit 2; }
# Read only the deployment selector. Dotenv is data, never shell code.
mode=${FLAGMAN_MODE:-auto}
if [ "$mode" = auto ]; then
 mode=$(sed -n 's/^[[:space:]]*DEPLOY_MODE[[:space:]]*=[[:space:]]*//p' "$env_file" | tail -n 1 | tr -d '\r')
 case "$mode" in ''|local|development) mode=local;; vps|'"vps"'|"'vps'") mode=vps;; *) echo 'DEPLOY_MODE must be local or vps' >&2; exit 2;; esac
fi
case "$mode" in
 local) compose_file="$root/infrastructure/compose.yaml";;
 vps) compose_file="$root/infrastructure/compose.vps.yaml";;
 *) echo 'MODE must be auto, local or vps' >&2; exit 2;;
esac
dc() {
 if [ "$mode" = local ]; then
  docker compose --env-file "$env_file" -f "$compose_file" -f "$root/infrastructure/compose.local.yaml" "$@"
 else
  docker compose --env-file "$env_file" -f "$compose_file" "$@"
 fi
}
migrate() {
 if [ "$mode" = vps ]; then
  dc up -d --wait --wait-timeout 120 db
  dc run --rm --no-deps migrate "$1"
 else
  goose -env "$env_file" -dir "$root/backend/database/migrations" "$1"
 fi
}
seed_users() {
 if [ "$mode" = vps ]; then
  dc --profile ops build seed-users
  dc --profile ops run --rm --no-deps seed-users
 else
  (cd "$root/backend"; go run ./cmd/seed-users -env "$env_file")
 fi
}
case "${1:-}" in
 up)
  dc config --quiet
  if [ "$mode" = vps ]; then
   dc run --rm --no-deps validate-config
  fi
  dc stop frontend backend
  dc up -d --wait --wait-timeout 120 db
  migrate up
  if [ "$mode" = vps ]; then
   # Force the role initialization to run after any new migrations.
   dc run --rm --no-deps monitoring-init
   dc up -d --build --wait --wait-timeout 180 traefik backend frontend prometheus grafana
  else
   dc up -d --build traefik backend frontend
  fi
  ;;
 down) dc down;;
 logs) dc logs -f;;
 migrate) migrate up;;
 migrate-status) migrate status;;
 migrate-version) migrate version;;
 migrate-validate) migrate validate;;
 migrate-one|rollback-one|rollback)
  [ "$mode" = local ] || { echo 'Single-step migration and rollback tasks are local-only; use task migrate on VPS' >&2; exit 2; }
  case "$1" in migrate-one) migrate up-by-one;; rollback-one) migrate down;; rollback) migrate reset;; esac
  ;;
 seed|seed-users)
  dc up -d --wait --wait-timeout 120 db
  migrate up
  seed_users
  if [ "$1" = seed ]; then
   for sql in 001_catalog.sql 002_inventory.sql; do
    dc exec -T db sh -c 'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$root/backend/database/seeds/$sql"
   done
  fi
  ;;
 *) echo 'Unknown deployment task' >&2; exit 2;;
esac
