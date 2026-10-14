#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
repo=$(cd -- "$root/.." && pwd)
for tool in docker goose python3; do
  command -v "$tool" >/dev/null || { printf 'Required tool missing: %s\n' "$tool" >&2; exit 1; }
done
endpoint=${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}')}
[[ "$endpoint" == unix://* ]] || { echo 'Acceptance requires a local Docker socket' >&2; exit 1; }
umask 077
export FRONTEND_TEST_DIR FRONTEND_TEST_PROJECT FRONTEND_TEST_ORIGIN FRONTEND_TEST_PASSWORD FRONTEND_TEST_SEED_DSN FRONTEND_TEST_WEB_PORT
FRONTEND_TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/flagman-frontend.XXXXXXXX")
FRONTEND_TEST_PROJECT="flagman-frontend-$(python3 -c 'import secrets; print(secrets.token_hex(6))')"
FRONTEND_TEST_ORIGIN=http://127.0.0.1
FRONTEND_TEST_WEB_PORT=0
FRONTEND_TEST_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
FRONTEND_TEST_SEED_DSN="postgres://flagman_frontend:$FRONTEND_TEST_PASSWORD@db:5432/flagman_frontend_test?sslmode=disable"
printf '%s' "$FRONTEND_TEST_PASSWORD" > "$FRONTEND_TEST_DIR/password"
printf '%s' "$FRONTEND_TEST_SEED_DSN" > "$FRONTEND_TEST_DIR/postgres_url"
touch "$FRONTEND_TEST_DIR/empty.env"
compose=(docker compose --env-file "$FRONTEND_TEST_DIR/empty.env" -p "$FRONTEND_TEST_PROJECT" -f "$root/tests/production/compose.yaml")
cleanup() {
  local status=$?
  trap - EXIT
  "${compose[@]}" --profile seed down --volumes --remove-orphans --timeout 5 >/dev/null 2>&1 || status=1
  for target in backend frontend seed-users; do
    if docker image inspect "$FRONTEND_TEST_PROJECT:$target" >/dev/null 2>&1; then
      docker image rm "$FRONTEND_TEST_PROJECT:$target" >/dev/null 2>&1 || status=1
    fi
  done
  rm -rf -- "$FRONTEND_TEST_DIR"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export BUILDX_CONFIG="$FRONTEND_TEST_DIR/buildx"
mkdir -p "$BUILDX_CONFIG"
docker build -f "$repo/infrastructure/Dockerfile.frontend" -t "$FRONTEND_TEST_PROJECT:frontend" "$repo"
docker build -f "$repo/infrastructure/Dockerfile.backend" -t "$FRONTEND_TEST_PROJECT:backend" "$repo/backend"
docker build --target seed-users -f "$repo/infrastructure/Dockerfile.backend" -t "$FRONTEND_TEST_PROJECT:seed-users" "$repo/backend"
"${compose[@]}" up -d --wait --wait-timeout 60 db
db_address=$("${compose[@]}" port db 5432)
[[ "$db_address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { echo 'Unexpected test database binding' >&2; exit 1; }
export GOOSE_DRIVER=postgres GOOSE_DBSTRING="postgres://flagman_frontend:$FRONTEND_TEST_PASSWORD@$db_address/flagman_frontend_test?sslmode=disable"
unset GOOSE_TABLE
if ! goose -env=none -dir "$repo/backend/database/migrations" up > "$FRONTEND_TEST_DIR/migrations.log" 2>&1; then
  echo 'Isolated acceptance migrations failed' >&2; exit 1
fi
"${compose[@]}" exec -T db psql -X -v ON_ERROR_STOP=1 -U flagman_frontend -d flagman_frontend_test \
  < "$repo/infrastructure/demo/settings.sql" > "$FRONTEND_TEST_DIR/settings.log" 2>&1
if ! "${compose[@]}" --profile seed run --rm seed-users > "$FRONTEND_TEST_DIR/seed.log" 2>&1; then
  echo 'Isolated demo-user seed failed' >&2; exit 1
fi
"${compose[@]}" up -d backend frontend traefik
web_address=$("${compose[@]}" port traefik 80)
[[ "$web_address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { echo 'Unexpected test frontend binding' >&2; exit 1; }
FRONTEND_TEST_ORIGIN="http://$web_address"
FRONTEND_TEST_WEB_PORT=${web_address##*:}
# Pin the allocated port before recreation: Docker can choose a new random port
# on restart. Backend Origin and the browser must retain the same exact address.
"${compose[@]}" up -d backend traefik
# Re-resolve nginx's upstream after recreating the backend with the exact Origin.
"${compose[@]}" restart frontend
[[ $("${compose[@]}" port traefik 80) == "$web_address" ]] || { echo 'Test origin changed during restart' >&2; exit 1; }
python3 - "$FRONTEND_TEST_ORIGIN" <<'PY'
import sys
import time
import urllib.request
for _ in range(30):
    try:
        with urllib.request.urlopen(sys.argv[1] + '/api/v1/ready', timeout=1) as response:
            if response.status == 200:
                break
    except OSError:
        pass
    time.sleep(1)
else:
    sys.exit('Isolated Traefik/nginx/API readiness failed')
PY
cd "$root"
npx playwright test --config playwright.production.config.ts "$@"
