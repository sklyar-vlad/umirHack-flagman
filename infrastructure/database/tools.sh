#!/bin/sh
set -eu
# The password never appears in process arguments or operator output.
PGPASSWORD=$(cat /run/secrets/postgres_password)
export PGPASSWORD
exec "$@"
