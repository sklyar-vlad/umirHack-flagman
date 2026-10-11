#!/bin/sh
set -eu
# Single-variable expansion in provisioning preserves literal dollars in passwords.
FLAGMAN_GRAFANA_PASSWORD=$(cat /run/secrets/grafana_pg_password)
export FLAGMAN_GRAFANA_PASSWORD
exec /run.sh
