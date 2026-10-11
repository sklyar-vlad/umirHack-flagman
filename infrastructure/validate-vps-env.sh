#!/bin/sh
set -eu
LC_ALL=C
export LC_ALL
if [ "${#METRICS_TOKEN}" -lt 32 ]; then
 echo 'METRICS_TOKEN must contain at least 32 bytes. Generate it with: openssl rand -hex 32' >&2
 exit 2
fi
printf '%s\n' "$FLAGMAN_DOMAIN" | grep -Eq '^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$' || {
 echo 'FLAGMAN_DOMAIN must be a DNS hostname without scheme, port or path' >&2
 exit 2
}
[ "${#FLAGMAN_DOMAIN}" -le 253 ] || { echo 'FLAGMAN_DOMAIN is too long' >&2; exit 2; }
