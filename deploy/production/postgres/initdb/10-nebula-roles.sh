#!/usr/bin/env bash
# Runs once, on an empty data directory (docker-entrypoint-initdb.d). Creates one role and one
# database per application plus a read-only monitoring role. Passwords come from secret files.
# The Nebula runtime role (DML only) is created by 20-nebula-runtime-role.sql, which deploy.sh
# also re-applies on every deploy (existing installations, drift repair).
set -euo pipefail

# secret NAME: prints the secret file's content without line breaks. Fails (and so aborts initdb,
# leaving no half-initialised cluster) when the file is missing or shorter than 16 characters:
# `set -e` does not apply inside the $(...) arguments below, so a missing file would otherwise
# create roles with an EMPTY password.
secret() {
	local v
	v="$(tr -d '\r\n' <"/run/secrets/$1")" || { echo "initdb: cannot read secret $1" >&2; exit 1; }
	((${#v} >= 16)) || { echo "initdb: secret $1 is empty or shorter than 16 characters" >&2; exit 1; }
	printf '%s' "$v"
}
nebula_pw="$(secret nebula_db_password)"
keycloak_pw="$(secret keycloak_db_password)"
monitor_pw="$(secret monitoring_db_password)"

psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres \
	-v nebula_pw="$nebula_pw" \
	-v keycloak_pw="$keycloak_pw" \
	-v monitor_pw="$monitor_pw" <<'SQL'
-- Application roles: no superuser, no role/db creation.
-- nebula owns the Nebula schema and is used ONLY by `nebula migrate` (ADR-0018).
CREATE ROLE nebula LOGIN PASSWORD :'nebula_pw' NOSUPERUSER NOCREATEDB NOCREATEROLE CONNECTION LIMIT 10;
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

# Query statistics for the monitoring role (pg_monitor may read them; query text is not exported).
psql -v ON_ERROR_STOP=1 --username postgres --dbname nebula -c 'CREATE EXTENSION IF NOT EXISTS pg_stat_statements'
