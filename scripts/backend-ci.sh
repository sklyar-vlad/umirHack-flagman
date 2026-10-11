#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
mode=${1:-integration}
case "$mode" in
    integration|smoke|demo) ;;
    *) printf 'Usage: %s [integration|smoke|demo]\n' "$0" >&2; exit 2 ;;
esac
demo_phase=${2:-all}
case "$demo_phase" in
    basic|admin|operations|all) ;;
    *) echo 'Unknown demo phase' >&2; exit 2 ;;
esac
for tool in docker goose go python3; do
    command -v "$tool" >/dev/null || { printf 'Required tool missing: %s\n' "$tool" >&2; exit 1; }
done
# Test containers must live on this machine, never on a remote deployment daemon.
endpoint=${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}')}
[[ "$endpoint" == unix://* ]] || { echo 'CI requires a local Unix Docker socket' >&2; exit 1; }

umask 077
export FLAGMAN_CI_DIR
FLAGMAN_CI_DIR=$(mktemp -d "${TMPDIR:-/tmp}/flagman-ci.XXXXXXXX")
project="flagman-ci-$(python3 -c 'import secrets; print(secrets.token_hex(6))')"
export FLAGMAN_CI_IMAGE="${project}:backend"
export FLAGMAN_CI_UID="$(id -u)" FLAGMAN_CI_GID="$(id -g)"
compose=(docker compose --env-file "$FLAGMAN_CI_DIR/compose.env" -p "$project" -f "$root/infrastructure/compose.ci.yaml")
if [[ "$mode" == demo ]]; then
    compose+=(-f "$root/infrastructure/compose.demo.yaml")
    export FLAGMAN_DEMO_PROJECT="$project"
fi
cleanup() {
    local status=$?
    trap - EXIT
    # Include profile-scoped services in the cleanup model, even on early failure.
    if ! "${compose[@]}" --profile smoke down --volumes --remove-orphans --timeout 10 >/dev/null 2>&1; then
        printf 'CI resource cleanup failed for project %s\n' "$project" >&2
        status=1
    fi
    if [[ "$mode" != integration ]]; then
        docker rm -f "$project-image-check" >/dev/null 2>&1 || true
        for image in "$FLAGMAN_CI_IMAGE" "$project:builder"; do
            if docker image inspect "$image" >/dev/null 2>&1; then
                docker image rm "$image" >/dev/null 2>&1 || status=1
            fi
        done
    fi
    rm -rf -- "$FLAGMAN_CI_DIR"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
printf 'FLAGMAN_CI_DIR=%s\n' "$FLAGMAN_CI_DIR" > "$FLAGMAN_CI_DIR/compose.env"
python3 - "$FLAGMAN_CI_DIR" <<'PY'
from pathlib import Path
import secrets
import sys
directory = Path(sys.argv[1])
password = secrets.token_hex(32)
(directory / 'postgres_password').write_text(password)
(directory / 'postgres_url').write_text(
    f'postgres://flagman_ci:{password}@db:5432/flagman_ci_test?sslmode=disable')
for name in ['metrics_token', 'grafana_admin_password', 'grafana_pg_password']:
    (directory / name).write_text(secrets.token_hex(32))
PY
echo 'Starting disposable PostgreSQL 18 on a random loopback port'
"${compose[@]}" up -d --wait --wait-timeout 60 db
address=$("${compose[@]}" port db 5432)
[[ "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { printf 'Unexpected CI database port binding: %q\n' "$address" >&2; exit 1; }
password=$(<"$FLAGMAN_CI_DIR/postgres_password")
test_url="postgres://flagman_ci:${password}@${address}/flagman_ci_test?sslmode=disable"
export GOOSE_DRIVER=postgres GOOSE_DBSTRING="$test_url"
export GOOSE_MIGRATION_DIR="$root/backend/database/migrations"
unset GOOSE_TABLE
# Explicitly disable dotenv. On failure suppress tool output that could include DSNs.
if ! goose -env=none -dir "$GOOSE_MIGRATION_DIR" up > "$FLAGMAN_CI_DIR/migrations.log" 2>&1; then
    echo 'CI forward migration failed in the disposable database' >&2
    exit 1
fi
echo 'Forward Goose migrations applied successfully'

if [[ "$mode" != integration ]]; then
    # Keep buildx activity/cache metadata writable and outside the checkout.
    export BUILDX_CONFIG="$FLAGMAN_CI_DIR/buildx"
    mkdir -p "$BUILDX_CONFIG"
    # Never copy real dotenv/certificates into even the temporary build context.
    # Synthetic canaries exercise the actual backend Docker ignore rules.
    python3 - "$root/backend" "$FLAGMAN_CI_DIR/context" <<'PY'
from pathlib import Path
import shutil
import sys
source, destination = map(Path, sys.argv[1:])
shutil.copytree(source, destination, ignore=shutil.ignore_patterns(
    '.git', '.beads', '.serena', '.env', '.env.*', '*.pem', '*.key', '*.dump', '*.log', '__pycache__'))
for name in ['.env', '.env.ci-canary', 'ci-canary.pem', 'ci-canary.key', 'ci-canary.dump']:
    (destination / name).write_text('synthetic-ci-secret-must-not-enter-build')
PY
    docker build --target build-stage -f "$root/infrastructure/Dockerfile.backend" -t "$project:builder" "$FLAGMAN_CI_DIR/context"
    docker run --rm --name "$project-image-check" --network none --entrypoint sh "$project:builder" -ec '
        for file in .env .env.ci-canary ci-canary.pem ci-canary.key ci-canary.dump; do
            test ! -e "/app/$file"
        done'
    docker build -f "$root/infrastructure/Dockerfile.backend" -t "$FLAGMAN_CI_IMAGE" "$FLAGMAN_CI_DIR/context"
    docker run --rm --name "$project-image-check" --network none --entrypoint sh "$FLAGMAN_CI_IMAGE" -ec '
        test -x /app/server
        test "$(ls -A /app)" = server'
    # The ordinary demo-user seeder loads only our empty, external CI env file.
    export DEMO_ADMIN_PASSWORD="$password" DEMO_MANAGER_PASSWORD="$password"
    export DEMO_STOREKEEPER_PASSWORD="$password" DEMO_ORDER_OPERATOR_PASSWORD="$password"
    if ! go -C backend run ./cmd/seed-users -env "$FLAGMAN_CI_DIR/compose.env" > "$FLAGMAN_CI_DIR/seed.log" 2>&1; then
        echo 'CI demo-user seeding failed in the disposable database' >&2
        exit 1
    fi
    seeds=()
    # The demonstration starts with users and migrations only. Its entire
    # catalog/inventory is created by ordinary API calls, not business SQL.
    if [[ "$mode" == smoke ]]; then seeds=(001_catalog.sql 002_inventory.sql); fi
    for seed in "${seeds[@]}"; do
        if ! "${compose[@]}" exec -T db psql -X -v ON_ERROR_STOP=1 -U flagman_ci -d flagman_ci_test \
            < "$root/backend/database/seeds/$seed" > "$FLAGMAN_CI_DIR/$seed.log" 2>&1; then
            echo 'CI catalog/inventory seeding failed in the disposable database' >&2
            exit 1
        fi
    done
    if [[ "$mode" == demo ]]; then
        # Singleton has no create API; this is the sole baseline fixture alongside
        # seed-users. Business catalog, batches and movements start empty.
        "${compose[@]}" exec -T db psql -X -v ON_ERROR_STOP=1 -U flagman_ci -d flagman_ci_test \
            < "$root/infrastructure/demo/settings.sql" > "$FLAGMAN_CI_DIR/settings.log" 2>&1
        export FLAGMAN_GRAFANA_PASSWORD
        FLAGMAN_GRAFANA_PASSWORD=$(<"$FLAGMAN_CI_DIR/grafana_pg_password")
        # Password is inherited in memory, never interpolated into SQL/arguments.
        "${compose[@]}" exec -T -e FLAGMAN_GRAFANA_PASSWORD db \
            psql -X -v ON_ERROR_STOP=1 -U flagman_ci -d flagman_ci_test \
            < "$root/infrastructure/monitoring/grafana-role.sql" > "$FLAGMAN_CI_DIR/grafana-role.log" 2>&1
    fi
    "${compose[@]}" --profile smoke up -d backend
    api_address=$("${compose[@]}" port backend 8080)
    [[ "$api_address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { echo 'Unexpected CI API port binding' >&2; exit 1; }
    if [[ "$mode" == demo ]]; then
        if [[ "$demo_phase" == operations || "$demo_phase" == all ]]; then
            "${compose[@]}" --profile smoke up -d prometheus grafana
            grafana_address=$("${compose[@]}" port grafana 3000)
            [[ "$grafana_address" =~ ^127\.0\.0\.1:[0-9]+$ ]] || { echo 'Unexpected demo Grafana binding' >&2; exit 1; }
            export FLAGMAN_DEMO_GRAFANA="http://$grafana_address"
        fi
        python3 -u "$root/scripts/demo-backend-api.py" "http://$api_address" "$demo_phase"
        "${compose[@]}" logs --no-color backend > "$FLAGMAN_CI_DIR/backend.log"
        python3 - "$FLAGMAN_CI_DIR" <<'PY'
from pathlib import Path
import sys
directory = Path(sys.argv[1])
log = (directory / 'backend.log').read_text()
for name in ['postgres_password', 'metrics_token', 'grafana_admin_password', 'grafana_pg_password']:
    if (directory / name).read_text() in log:
        sys.exit('Demo credential leaked into backend logs')
PY
        echo 'Clean ordinary-API backend demonstration passed'
        exit 0
    fi
    python3 "$root/scripts/check-backend-api.py" "http://$api_address" up
    # Verify real readiness failure without making liveness depend on PostgreSQL.
    "${compose[@]}" stop --timeout 5 db
    python3 "$root/scripts/check-backend-api.py" "http://$api_address" down
    "${compose[@]}" logs --no-color backend > "$FLAGMAN_CI_DIR/backend.log"
    if grep -Fq "$password" "$FLAGMAN_CI_DIR/backend.log"; then
        echo 'CI credentials leaked into backend logs' >&2
        exit 1
    fi
    echo 'Backend image, secret exclusion and isolated Compose API smoke passed'
    exit 0
fi

export FLAGMAN_TEST_DATABASE_URL="$test_url" PRODUCT_TEST_DATABASE_URL="$test_url"
export SUPPLY_TEST_DATABASE_URL="$test_url" MENU_TEST_DATABASE_URL="$test_url"
export BATCH_TEST_DATABASE_URL="$test_url" INVENTORY_TEST_DATABASE_URL="$test_url"
# Extra monitoring/TLS/backup suites use separately built proxy images. Keep this
# transaction job self-contained; Docker image/API verification is a separate task.
unset FLAGMAN_TEST_MONITORING FLAGMAN_TEST_BACKUP FLAGMAN_TEST_HTTPS FLAGMAN_TEST_SSE_PROXIES
result=0
go -C backend test -json -count=1 -p 4 -timeout 10m ./... > "$FLAGMAN_CI_DIR/tests.jsonl" 2>&1 || result=$?
python3 "$root/scripts/check-backend-test-results.py" "$FLAGMAN_CI_DIR/tests.jsonl" || result=1
exit "$result"
