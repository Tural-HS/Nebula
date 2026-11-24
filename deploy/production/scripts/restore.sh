#!/usr/bin/env bash
# DISASTER RECOVERY: restores the production cluster (nebula + keycloak databases) from S3.
# Destroys the current database contents. Rehearse with restore-test.sh first.
#
#   sudo ./scripts/restore.sh                                   # latest: newest backup + all archived WAL
#   sudo ./scripts/restore.sh --target '2026-12-01 09:59:00+00' # point in time (before the incident)
#   sudo ./scripts/restore.sh --set 20261201-020001F            # a specific backup, no WAL replay beyond it
#
# Flow: stop Nebula/Keycloak -> stop Postgres -> pgbackrest restore --delta into the data volume ->
# start Postgres (recovery, then promote) -> verify -> start the rest. A new timeline is created; the
# old one stays in the repository, so a wrong target can be retried.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

stanza=${PGBACKREST_STANZA:-nebula}
opts=()
while (($#)); do
	case $1 in
	--target) opts+=(--type=time "--target=$2" --target-action=promote); shift ;;
	--set) opts+=("--set=$2" --type=immediate --target-action=promote); shift ;;
	*) die "unknown argument $1" ;;
	esac
	shift
done

# Validate before anything is stopped: a typo must not take the stack down.
for ((i = 0; i < ${#opts[@]}; i++)); do
	case ${opts[i]} in
	--target=*) date -d "${opts[i]#--target=}" >/dev/null 2>&1 || die "invalid --target '${opts[i]#--target=}' (e.g. '2026-12-01 09:59:00+00')" ;;
	--set=*) [[ ${opts[i]#--set=} =~ ^[0-9]{8}-[0-9]{6}F(_[0-9]{8}-[0-9]{6}[DI])?$ ]] || die "invalid --set '${opts[i]#--set=}' (see backup.sh info)" ;;
	esac
done

take_lock 3600
compose exec -T postgres nebula-pgbackrest --stanza="$stanza" info >&2 || log "WARN: could not list backups (is postgres down?)"
confirm "This REPLACES the production databases nebula and keycloak (${opts[*]:-latest})."

log "stopping application services"
compose stop caddy nebula keycloak
log "stopping postgres"
compose stop postgres

log "restoring (delta)"
compose run --rm --no-deps -T postgres \
	nebula-pgbackrest --stanza="$stanza" --delta "${opts[@]}" restore

log "starting postgres (recovery)"
compose up -d --wait postgres
until [[ "$(psql_prod postgres 'SELECT pg_is_in_recovery()')" == f ]]; do sleep 5; done
log "recovered: schema $(schema_version), newest event $(psql_prod nebula 'SELECT max(occurred_at) FROM events')"

# A new timeline must be backed up before it can be used for PITR again.
"$DEPLOY_DIR/scripts/backup.sh" full --no-lock || log "WARN: post-restore full backup failed: fix before relying on backups"

log "starting the stack"
compose up -d --wait --wait-timeout 300 "${APP_SERVICES[@]}"
compose up -d
smoke_test
log "restore complete: verify the data with the team before announcing recovery"
