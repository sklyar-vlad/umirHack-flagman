#!/bin/sh
set -eu
umask 077
test "${#METRICS_TOKEN}" -ge 32 || { echo 'Metrics token requires at least 32 characters' >&2; exit 2; }
printf '%s' "$METRICS_TOKEN" > /tmp/flagman-metrics-token
exec /bin/prometheus --config.file=/etc/prometheus/prometheus.yaml --storage.tsdb.path=/prometheus --storage.tsdb.retention.time=15d
