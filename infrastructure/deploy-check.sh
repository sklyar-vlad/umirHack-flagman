#!/bin/sh
set -eu
: "${FLAGMAN_DOMAIN:?Set final DNS hostname without scheme, port or path}"
: "${FLAGMAN_SECRETS_DIR:?Set external secrets directory}"
: "${FLAGMAN_TLS_DIR:?Set external certificate directory}"
: "${FLAGMAN_BACKUP_DIR:?Set external backup directory}"
# DNS names only; prevent router expression injection or accidental URL input.
printf '%s\n' "$FLAGMAN_DOMAIN" | LC_ALL=C grep -Eq '^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$' || { echo 'Invalid domain' >&2; exit 2; }
[ "${#FLAGMAN_DOMAIN}" -le 253 ] || { echo 'Domain too long' >&2; exit 2; }
root=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
for directory in "$FLAGMAN_SECRETS_DIR" "$FLAGMAN_TLS_DIR" "$FLAGMAN_BACKUP_DIR"; do
 case "$directory" in /*) ;; *) echo 'Deployment directories must be absolute' >&2; exit 2;; esac
 resolved=$(realpath "$directory")
 case "$resolved" in "$root"|"$root"/*) echo 'Secrets, certificates and backups must be outside repository' >&2; exit 2;; esac
done
for name in postgres_password postgres_url metrics_token grafana_admin_password grafana_pg_password; do
 [ -s "$FLAGMAN_SECRETS_DIR/$name" ] || { echo "Missing secret file: $name" >&2; exit 2; }
 case "$(realpath "$FLAGMAN_SECRETS_DIR/$name")" in "$root"|"$root"/*) echo 'Secret symlink enters repository' >&2; exit 2;; esac
done
[ "$(wc -c < "$FLAGMAN_SECRETS_DIR/metrics_token")" -ge 32 ] || { echo 'Metrics token is too short' >&2; exit 2; }
for name in fullchain.pem privkey.pem; do
 case "$(realpath "$FLAGMAN_TLS_DIR/$name")" in "$root"|"$root"/*) echo 'Certificate symlink enters repository' >&2; exit 2;; esac
done
openssl x509 -in "$FLAGMAN_TLS_DIR/fullchain.pem" -noout -checkend 86400 >/dev/null
openssl verify -partial_chain -CAfile "$FLAGMAN_TLS_DIR/fullchain.pem" -verify_hostname "$FLAGMAN_DOMAIN" "$FLAGMAN_TLS_DIR/fullchain.pem" >/dev/null
certificate_key=$(openssl x509 -in "$FLAGMAN_TLS_DIR/fullchain.pem" -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256)
private_key=$(openssl pkey -in "$FLAGMAN_TLS_DIR/privkey.pem" -pubout -outform DER | openssl dgst -sha256)
[ "$certificate_key" = "$private_key" ] || { echo 'Certificate/key mismatch' >&2; exit 2; }
docker compose -f "$root/infrastructure/compose.https.yaml" config --quiet
printf 'Local deployment configuration and certificate checks passed. Remote rollout remains required.\n'
