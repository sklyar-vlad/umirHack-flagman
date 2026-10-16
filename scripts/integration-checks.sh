#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
mode=${1:-backend}
case "$mode" in backend|frontend) ;; *) echo 'Use backend or frontend' >&2; exit 2;; esac
for tool in docker python3; do
  command -v "$tool" >/dev/null || { echo "Missing tool: $tool" >&2; exit 1; }
done
endpoint=${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}')}
[[ "$endpoint" == unix://* ]] || { echo 'Tests require a local Docker socket' >&2; exit 1; }
umask 077
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/flagman-tests.XXXXXXXX")
project="flagman-test-$(python3 -c 'import secrets; print(secrets.token_hex(6))')"
password=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
unset POSTGRES_URL POSTGRES_DB POSTGRES_USER POSTGRES_PASS POSTGRES_VOLUME
unset HTTP_BIND HTTP_PORT POSTGRES_PORT GRAFANA_PORT PROMETHEUS_PORT TRAEFIK_PORT FRONTEND_ORIGIN
unset FLAGMAN_DOMAIN METRICS_TOKEN GRAFANA_ADMIN_USER GRAFANA_ADMIN_PASSWORD GRAFANA_PG_PASSWORD ENV
unset DEMO_ADMIN_PASSWORD DEMO_MANAGER_PASSWORD DEMO_STOREKEEPER_PASSWORD DEMO_ORDER_OPERATOR_PASSWORD
export COMPOSE_PROJECT_NAME="$project"
export BUILDX_CONFIG="$test_dir/buildx"
mkdir -p "$BUILDX_CONFIG"
cat > "$test_dir/env" <<EOF
DEPLOY_MODE=local
COMPOSE_PROJECT_NAME=$project
FLAGMAN_DOMAIN=127.0.0.1
POSTGRES_DB=flagman_test
POSTGRES_USER=flagman_test
POSTGRES_PASS=$password
POSTGRES_URL=postgres://flagman_test:$password@db:5432/flagman_test?sslmode=disable
POSTGRES_VOLUME=$project-postgres
HTTP_BIND=127.0.0.1
HTTP_PORT=0
POSTGRES_PORT=0
GRAFANA_PORT=0
PROMETHEUS_PORT=0
TRAEFIK_PORT=0
METRICS_TOKEN=$password
GRAFANA_ADMIN_PASSWORD=$password
GRAFANA_PG_PASSWORD=$password
DEMO_ADMIN_PASSWORD=$password
DEMO_MANAGER_PASSWORD=$password
DEMO_STOREKEEPER_PASSWORD=$password
DEMO_ORDER_OPERATOR_PASSWORD=$password
EOF
dc() { docker compose --env-file "$test_dir/env" -p "$project" -f "$root/infrastructure/compose.local.yaml" "$@"; }
cleanup() {
  local status=$?
  trap - EXIT
  dc --profile tools down --volumes --remove-orphans --timeout 5 >/dev/null 2>&1 || status=1
  for target in backend frontend migrate seed; do
    if docker image inspect "$project-$target" >/dev/null 2>&1; then
      docker image rm "$project-$target" >/dev/null 2>&1 || status=1
    fi
  done
  rm -rf -- "$test_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
dc up -d --wait --wait-timeout 120 db
dc run --build --rm migrate
address=$(dc port db 5432)
[[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { printf 'Unexpected test database binding: <%s>\n' "$address" >&2; exit 1; }
if [[ "$mode" == backend ]]; then
  export FLAGMAN_TEST_DATABASE_URL="postgres://flagman_test:$password@$address/flagman_test?sslmode=disable"
  export PRODUCT_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL" SUPPLY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
  export MENU_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL" BATCH_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
  export INVENTORY_TEST_DATABASE_URL="$FLAGMAN_TEST_DATABASE_URL"
  go -C backend test ./... -count=1
else
  if [[ -z ${PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH:-} ]] && command -v chromium >/dev/null; then
    export PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
    PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=$(command -v chromium)
  fi
  FLAGMAN_ENV_FILE="$test_dir/env" sh "$root/infrastructure/manage.sh" seed
  FLAGMAN_ENV_FILE="$test_dir/env" sh "$root/infrastructure/manage.sh" up
  web_address=$(dc port traefik 80)
  [[ "$web_address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { printf 'Unexpected test frontend binding: <%s>\n' "$web_address" >&2; exit 1; }
  export FRONTEND_TEST_ORIGIN="http://$web_address" FRONTEND_TEST_PASSWORD="$password"
  printf '\nHTTP_PORT=%s\nFRONTEND_ORIGIN=%s\n' "${web_address##*:}" "$FRONTEND_TEST_ORIGIN" >> "$test_dir/env"
  dc up -d --wait --wait-timeout 120 backend traefik
  dc restart frontend
  [[ $(dc port traefik 80) == "$web_address" ]] || { echo 'Test origin changed' >&2; exit 1; }
  cd frontend
  npx playwright test --config playwright.production.config.ts
fi
