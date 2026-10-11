# HTTPS deployment on VPS

This is the versioned rollout procedure. Local verification does not deploy a VPS
or verify its final domain. The owner performs the remote steps below afterwards.
Use the standalone `compose.https.yaml`; do not combine it with the HTTP/local
Compose files. It creates separate networks and a separate PostgreSQL volume.
It does not silently reuse the development database.

## Prerequisites and external configuration

Install Docker with Compose, OpenSSL and coreutils. Create DNS A/AAAA records for
one domain pointing to the VPS. Open TCP 80/443 and the SSH port in the host firewall.
Do not expose 5432, 3000 or 8081. Use an SSH tunnel for Grafana and Traefik admin.
Obtain a trusted TLS certificate using your certificate provider/ACME client
outside this repository; include intermediates in `fullchain.pem`.

Export these nonsecret variables in the operator shell or an external deployment
environment file (never copy the workflow environment into this repository):

```sh
export FLAGMAN_DOMAIN=warehouse.example.com
export POSTGRES_DB=flagman
export POSTGRES_USER=flagman_owner
export GRAFANA_ADMIN_USER=admin
export FLAGMAN_SECRETS_DIR=/srv/flagman/secrets
export FLAGMAN_TLS_DIR=/srv/flagman/tls
export FLAGMAN_BACKUP_DIR=/srv/flagman/backups
install -d -m 0700 "$FLAGMAN_SECRETS_DIR" "$FLAGMAN_TLS_DIR" "$FLAGMAN_BACKUP_DIR"
```

Create these external files using a password manager or secure editor:

| File | Content |
| --- | --- |
| `secrets/postgres_password` | Strong PostgreSQL password for the selected owner |
| `secrets/postgres_url` | `postgres://flagman_owner:<URL-encoded-password>@db:5432/flagman?sslmode=disable` with the exact selected user/database |
| `secrets/metrics_token` | At least 32 random bytes, preferably 64 hex characters |
| `secrets/grafana_admin_password` | Strong initial Grafana admin password |
| `secrets/grafana_pg_password` | Independent password for SELECT-only `flagman_grafana` |
| `tls/fullchain.pem` | Trusted certificate chain for `FLAGMAN_DOMAIN` |
| `tls/privkey.pem` | Matching unencrypted private key; protect access |

Compose mounts only the needed files into each service. PostgreSQL uses
`POSTGRES_PASSWORD_FILE`, backend uses `POSTGRES_URL_FILE`/`METRICS_TOKEN_FILE`,
and Grafana uses its password file and the warehouse datasource entrypoint.
The host directories must stay 0700 and owned by the operator. For nonroot
Grafana (UID 472)/Prometheus (65534), files must be readable by their container
UID: configure ACL/group ownership or use 0444 files inside the protected host
directory. Compose file-backed secrets do not remap ownership automatically.
Mounting single files exposes them to the designated container, without opening
the host directory. Never place these files in a build context, image, Git or
command arguments. Changing an initialized DB password requires explicit database
rotation as well as updating both files; changing the initialization file alone
does not rotate it. Existing Grafana admin credentials likewise need normal rotation.

Run from the repository root:

```sh
sh infrastructure/deploy-check.sh
# Keep this alias in the operator shell, not in Git.
https_compose() { docker compose -f infrastructure/compose.https.yaml "$@"; }
https_compose build backend frontend
https_compose up -d --wait --wait-timeout 60 db
https_compose --profile ops run --rm migrate
```

`deploy-check.sh` verifies external paths, required secrets, domain, certificate
hostname/expiry/key and the rendered Compose without printing secrets. Verify the
trusted chain separately with your provider; a self-signed certificate is suitable
only for isolated local tests. The migration tool receives its URL from a secret
file through environment, never an argument. The migration operation is forward
only. Before upgrading an existing installation take a backup as shown below.
Provision application accounts through the project's approved account bootstrap;
production is not automatically populated with demo stock/users. Ensure the first
admin account/password are available before opening the UI to users.

Set up the reporting role after migrations. This command reads only its separate
password file and runs the versioned least-privilege SQL inside the database container:

```sh
https_compose exec -T db sh -c '
  export FLAGMAN_GRAFANA_PASSWORD="$(cat /run/secrets/grafana_pg_password)"
  exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"
' < infrastructure/monitoring/grafana-role.sql
https_compose up -d backend frontend traefik prometheus grafana
```

No application DB port is published. Backend and Grafana reach the database over
an internal network; datasource credentials cannot read users, sessions or audit.
Grafana requires login on loopback 3000; Traefik's admin API exists only on the
separate loopback 8081 entrypoint. Prometheus and `/metrics` have no public route.
The public domain routes only to nginx; `/api/` is forwarded without stripping its
prefix. The exact Origin is `https://$FLAGMAN_DOMAIN` (no slash or explicit :443).
Backend issues host-only HttpOnly SameSite=Lax Secure cookies even though the
internal proxy hop is HTTP. Credentialed CORS permits only this exact origin.

## Backup, restore and upgrades

Run the operations profile against the selected database; archives stay outside
source on the host. It uses PostgreSQL 18 clients and passwords from file:

```sh
https_compose --profile ops run --rm db-tools sh /scripts/backup.sh /backup/warehouse-20261011.dump
https_compose --profile ops run --rm db-tools sh /scripts/restore-test.sh /backup/warehouse-20261011.dump flagman_restore_test_20261011
```

Backup is a consistent custom `pg_dump` archive. Existing archive paths and
existing restore database names are refused. Restore uses a fresh test database,
`pg_restore --single-transaction --exit-on-error --no-owner --no-privileges`, then
queries migration history, every table count and all reporting views. It never
cleans/replaces the application DB. Review stored business facts, Goose state and
counts from the backup snapshot; later writes in the running source are expected
to differ. To check restored readiness, run a separate backend attached only to
the isolated warehouse network with an external `postgres_url` pointing to the
test database, then query `/api/v1/ready` inside that network. Reapply the Grafana
role script to the restored database before querying it with that role. Cluster
roles, passwords and external certificates require their own secure backup.

Store encrypted archive copies off the VPS. Choose retention and a scheduler for
the workload, and perform restore drills. Neither secrets nor backups belong in
Git. Before updates: backup and verify; stop backend/frontend, apply forward
migrations, rebuild/start, then run the checks below. Keep the old archive/image
until verification passes. Never run automatic `down -v`, schema resets or
migration rollback as an upgrade procedure. Remote switching/restoring a working
DB and removing drill databases are explicit operator actions.

Certificate renewal must replace the external chain/key, re-run deploy-check,
and recreate Traefik so Compose refreshes the mounted files. Verify the new expiry
and chain with ordinary curl afterwards. Keep application clients on HTTPS.

## Verification by the owner after rollout

```sh
curl --fail --show-error --silent "https://$FLAGMAN_DOMAIN/api/v1/healthcheck"
curl --fail --show-error --silent "https://$FLAGMAN_DOMAIN/api/v1/ready"
curl --head "http://$FLAGMAN_DOMAIN/api/v1/healthcheck"
https_compose ps
# Technical ports must be bound only to 127.0.0.1 or absent:
ss -lnt
ssh -L 3000:127.0.0.1:3000 -L 8081:127.0.0.1:8081 user@vps
```

Do not use `curl -k` for the final domain. HTTP must redirect to HTTPS, readiness
must return 200 and liveness must remain independent of database availability.
From a machine outside the VPS confirm 5432/3000/8081 are unreachable. Public
`/metrics`, `/dashboard/` and `/api/rawdata` must not expose metrics/admin data.
Through the SSH tunnel open Grafana, authenticate and check the provisioned
Flagman dashboard: HTTP, workers/outbox and all four SQL panels. Inspect the
Prometheus target through the private network if needed; `up` should be 1.

Log in using the browser Network panel. Confirm `Set-Cookie` contains Secure,
HttpOnly, SameSite=Lax, Path=/api/v1 and no Domain. Confirm allowed responses have
`Access-Control-Allow-Origin: https://<exact-domain>` and credentials=true, while
another Origin is refused for login/mutations and gets no CORS authorization.
Mutations require the returned CSRF token. For a terminal stream test, store a
login JSON payload and curl config/cookie jar under a protected external directory
(0600), send login with the exact Origin and no password in argv, then:

```sh
curl --fail --show-error --silent --config /srv/flagman/operator/login.curl
curl -N --cookie /srv/flagman/operator/cookies.txt "https://$FLAGMAN_DOMAIN/api/v1/events"
```

The first `: connected` comment arrives immediately, heartbeat every 20 seconds,
and inventory changes arrive without proxy buffering. Traefik uses immediate
flush/no whole-response write timeout; nginx disables buffering/cache/gzip for
this exact SSE route. Reconnect with Last-Event-ID to verify replay; logout or
revoke the test session afterwards. Monitor safe backend logs and worker metrics.
Record rollout/domain verification separately; this repository's local checks
are not evidence of a remote deployment.

## Local evidence

```sh
FLAGMAN_TEST_HTTPS=1 FLAGMAN_TEST_MONITORING=1 FLAGMAN_TEST_BACKUP=1 \
  FLAGMAN_TEST_SSE_PROXIES=1 go -C backend test -count=1 ./...
go -C backend vet ./...
go -C backend build ./...
```

Use only the inherited dedicated local test URLs. Container tests render the
actual HTTPS Compose, verify a trusted local TLS handshake, HTTP redirect,
unknown-SNI refusal, real authenticated PostgreSQL sessions through Traefik/nginx,
Secure cookie/CSRF/exact CORS, unbuffered SSE and private admin/metrics routing.
Backup tests compare every stored table/view fact and sequences in fresh isolated
test databases. No production/VPS is accessed by these checks.
