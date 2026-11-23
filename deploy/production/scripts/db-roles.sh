#!/usr/bin/env bash
# Applies postgres/initdb/20-nebula-runtime-role.sql to the running cluster: creates the runtime
# role nebula_app if missing, sets its password from the secret file, re-asserts its attributes
# and removes the obsolete monitoring function. Idempotent; deploy.sh runs it on every deploy.
# Table privileges come from `nebula migrate up` (NEBULA_DB_RUNTIME_ROLE), i.e. the next deploy.
#
#   sudo ./scripts/db-roles.sh
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

compose ps --status running -q postgres | grep -q . || die "postgres is not running"
ensure_db_roles
log "database roles ok"
