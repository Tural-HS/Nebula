-- Nebula runtime role (ADR-0018): what `nebula serve` connects as. LOGIN and CONNECT only;
-- table and sequence privileges (DML, no DDL) are granted by `nebula migrate up` with
-- NEBULA_DB_RUNTIME_ROLE=nebula_app, including default privileges for future migrations.
--
-- Idempotent. Runs at initdb (docker-entrypoint-initdb.d, as postgres) and on every deploy:
--   docker compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -d postgres -f - < postgres/initdb/20-nebula-runtime-role.sql
-- The password is read from the secret file by psql itself (never on a command line) and the
-- statements that carry it are kept out of the server log, even on error.

\set ON_ERROR_STOP on
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nebula_app') AS create_app \gset
\if :create_app
CREATE ROLE nebula_app;
\endif
-- Re-asserted every time: repairs drift (manual ALTER ROLE ... SUPERUSER etc.).
ALTER ROLE nebula_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT CONNECTION LIMIT 60;
-- The migration role needs only a few connections (migrate uses 2).
ALTER ROLE nebula CONNECTION LIMIT 10;

\set app_pw `cat /run/secrets/nebula_app_db_password`
BEGIN;
SET LOCAL log_statement = 'none';
SET LOCAL log_min_error_statement = 'panic';
SET LOCAL log_min_duration_statement = -1;
SELECT length(:'app_pw') >= 16 AS pw_ok \gset
\if :pw_ok
ALTER ROLE nebula_app PASSWORD :'app_pw';
\else
DO $$ BEGIN RAISE EXCEPTION 'secret nebula_app_db_password is missing or shorter than 16 characters'; END $$;
\endif
COMMIT;
\unset app_pw

GRANT CONNECT ON DATABASE nebula TO nebula_app;

\connect nebula
-- PostgreSQL >= 15 already denies CREATE on public to PUBLIC; stated explicitly.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
-- Superseded by the app's own job-queue metrics (ADR-0018): remove the SECURITY DEFINER workaround.
DROP FUNCTION IF EXISTS monitoring.job_queue_stats();
DROP SCHEMA IF EXISTS monitoring;
