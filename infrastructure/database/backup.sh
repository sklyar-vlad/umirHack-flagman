#!/bin/sh
set -eu
umask 077
: "${PGDATABASE:?Set the source database explicitly}"
[ "$#" -eq 1 ] || { echo "Usage: backup.sh /external/path/archive.dump" >&2; exit 2; }
archive=$1
case "$archive" in /*) ;; *) echo "Archive must use an absolute external path" >&2; exit 2;; esac
# noclobber protects an existing backup. A partial archive is removed on error.
set -C
exec 3>"$archive"
trap 'rm -f -- "$archive"' EXIT HUP INT TERM
pg_dump --format=custom --no-password >&3
exec 3>&-
pg_restore --list "$archive" >/dev/null
trap - EXIT HUP INT TERM
printf 'Backup verified: %s\n' "$archive"
