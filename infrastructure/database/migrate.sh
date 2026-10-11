#!/bin/sh
set -eu
action=${1:-up}
[ "$#" -le 1 ] || { echo 'One migration action expected' >&2; exit 2; }
case "$action" in up|status|version|validate) ;; *) echo 'Only forward migrations and inspection are supported' >&2; exit 2;; esac
GOOSE_DBSTRING=$(cat /run/secrets/postgres_url)
[ -n "$GOOSE_DBSTRING" ] || { echo 'Empty database secret' >&2; exit 2; }
export GOOSE_DBSTRING
export GOOSE_DRIVER=postgres
exec goose -dir /migrations "$action"
