#!/bin/sh
set -eu
: "${PGDATABASE:?Set the source database explicitly}"
[ "$#" -eq 2 ] || { echo "Usage: restore-test.sh archive.dump flagman_restore_test_NAME" >&2; exit 2; }
archive=$1
target=$2
case "$target" in flagman_restore_test_?*) ;; *) echo "Only a fresh flagman_restore_test_ database is allowed" >&2; exit 2;; esac
case "$target" in *[!a-z0-9_]*) echo "Invalid test database name" >&2; exit 2;; esac
[ "$target" != "$PGDATABASE" ] || { echo "Target must differ from source" >&2; exit 2; }
pg_restore --list "$archive" >/dev/null
# No --clean, no overwrite, no destructive migrations; existing names fail.
createdb --no-password --maintenance-db="$PGDATABASE" --template=template0 "$target"
pg_restore --no-password --dbname="$target" --single-transaction --exit-on-error \
  --no-owner --no-privileges "$archive"
PGDATABASE="$target" psql -X --no-password -v ON_ERROR_STOP=1 \
  -f "$(dirname "$0")/verify-restore.sql"
printf 'Restored and verified isolated database: %s\n' "$target"
