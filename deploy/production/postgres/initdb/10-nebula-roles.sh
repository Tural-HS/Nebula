#!/usr/bin/env bash
# Runs once, on an empty data directory (docker-entrypoint-initdb.d). Creates one role and one
# database per application plus a read-only monitoring role. Passwords come from secret files.
set -euo pipefail

secret() { tr -d '\r\n' <"/run/secrets/$1"; }

psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres \
	-v nebula_pw="$(secret nebula_db_password)" \
	-v keycloak_pw="$(secret keycloak_db_password)" \
	-v monitor_pw="$(secret monitoring_db_password)" <<'SQL'
-- Application roles: no superuser, no role/db creation.
CREATE ROLE nebula LOGIN PASSWORD :'nebula_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE CONNECTION LIMIT 60;
CREATE ROLE keycloak LOGIN PASSWORD :'keycloak_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE CONNECTION LIMIT 40;
-- Monitoring: statistics views only (pg_monitor), no table data.
CREATE ROLE nebula_monitor LOGIN PASSWORD :'monitor_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE CONNECTION LIMIT 5;
GRANT pg_monitor TO nebula_monitor;

CREATE DATABASE nebula OWNER nebula;
CREATE DATABASE keycloak OWNER keycloak;
REVOKE ALL ON DATABASE nebula FROM PUBLIC;
REVOKE ALL ON DATABASE keycloak FROM PUBLIC;
GRANT CONNECT ON DATABASE nebula TO nebula_monitor;
GRANT CONNECT ON DATABASE postgres TO nebula_monitor;

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
SQL

# Job-queue statistics for alerting without granting the monitoring role any table access:
# a SECURITY DEFINER function owned by the application role. PL/pgSQL is late-bound, so the
# function can exist before the migrations create the jobs table.
psql -v ON_ERROR_STOP=1 --username postgres --dbname nebula <<'SQL'
-- Query statistics for the monitoring role (pg_monitor may read them; query text is not exported).
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
CREATE SCHEMA monitoring AUTHORIZATION nebula;
GRANT USAGE ON SCHEMA monitoring TO nebula_monitor;
SET ROLE nebula;
CREATE FUNCTION monitoring.job_queue_stats()
	RETURNS TABLE (state text, jobs bigint, oldest_age_seconds double precision)
	LANGUAGE plpgsql STABLE SECURITY DEFINER
	SET search_path = pg_catalog, public
AS $fn$
BEGIN
	IF to_regclass('public.jobs') IS NULL THEN
		RETURN; -- before the first migration
	END IF;
	RETURN QUERY
		SELECT 'pending_due'::text, count(*),
		       coalesce(extract(epoch FROM now() - min(j.run_at)), 0)::double precision
		FROM public.jobs j WHERE j.state = 'pending' AND j.run_at <= now()
		UNION ALL
		SELECT 'failed_last_hour'::text, count(*), 0::double precision
		FROM public.jobs j WHERE j.state = 'failed' AND j.finished_at > now() - interval '1 hour';
END
$fn$;
REVOKE ALL ON FUNCTION monitoring.job_queue_stats() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION monitoring.job_queue_stats() TO nebula_monitor;
RESET ROLE;
SQL
