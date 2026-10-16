#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root/backend"

format() {
    local files
    files=$(gofmt -l .)
    if [[ -n "$files" ]]; then
        printf 'Go files need formatting:\n%s\n' "$files" >&2
        return 1
    fi
}

unit() (
    unset FLAGMAN_TEST_DATABASE_URL PRODUCT_TEST_DATABASE_URL SUPPLY_TEST_DATABASE_URL
    unset MENU_TEST_DATABASE_URL BATCH_TEST_DATABASE_URL INVENTORY_TEST_DATABASE_URL
    unset FLAGMAN_TEST_MONITORING FLAGMAN_TEST_BACKUP FLAGMAN_TEST_HTTPS FLAGMAN_TEST_SSE_PROXIES
    go test -count=1 ./...
)

case "${1:-all}" in
    format) format ;;
    lint) golangci-lint run --timeout=5m ;;
    unit) unit ;;
    vet) go vet ./... ;;
    build) go build ./... ;;
    all)
        format
        golangci-lint run --timeout=5m
        unit
        go vet ./...
        go build ./...
        ;;
    *) printf 'Usage: %s [all|format|lint|unit|vet|build]\n' "$0" >&2; exit 2 ;;
esac
