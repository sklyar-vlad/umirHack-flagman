\set ON_ERROR_STOP on
\getenv grafana_password FLAGMAN_GRAFANA_PASSWORD
BEGIN;
SELECT 'CREATE ROLE flagman_grafana LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS(SELECT FROM pg_roles WHERE rolname='flagman_grafana') \gexec
-- Refuse to reuse a role with inherited/application privileges.
SELECT count(*)=0 AS safe_role FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname='flagman_grafana' \gset
\if :safe_role
\else
\echo 'flagman_grafana has role memberships; refusing setup'
ROLLBACK;
\quit 1
\endif
ALTER ROLE flagman_grafana NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
SELECT format('ALTER ROLE flagman_grafana PASSWORD %L', :'grafana_password') \gexec
ALTER ROLE flagman_grafana SET default_transaction_read_only=on;
ALTER ROLE flagman_grafana SET statement_timeout='5s';
ALTER ROLE flagman_grafana SET idle_in_transaction_session_timeout='5s';
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM flagman_grafana;
REVOKE ALL ON SCHEMA public FROM flagman_grafana;
GRANT USAGE ON SCHEMA public TO flagman_grafana;
GRANT SELECT ON monitoring_stock,monitoring_problem_batches,monitoring_stop_list,monitoring_losses TO flagman_grafana;
COMMIT;
