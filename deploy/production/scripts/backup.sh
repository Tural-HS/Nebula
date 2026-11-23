#!/usr/bin/env bash
# pgBackRest operations for the whole cluster (databases nebula + keycloak), plus metrics for alerting.
#
#   sudo ./scripts/backup.sh init        # first install: create the stanza in the S3 repo and check archiving
#   sudo ./scripts/backup.sh full|diff|incr
#   sudo ./scripts/backup.sh info
#   sudo ./scripts/backup.sh check       # verifies WAL archiving end to end
#
# Scheduled by systemd/nebula-backup-*.timer. Writes $NEBULA_STATE_DIR/textfile/pgbackrest.prom.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

cmd=${1:-}
lock=1
[[ ${2:-} == --no-lock ]] && lock=0
stanza=${PGBACKREST_STANZA:-nebula}

pgbr() { compose exec -T postgres nebula-pgbackrest --stanza="$stanza" "$@"; }

emit_metrics() { # emit_metrics last_run_ok type duration
	local info
	info="$(pgbr info --output=json 2>/dev/null || echo '[]')"
	{
		echo '# HELP nebula_pgbackrest_backup_last_run_success 1 if the last scheduled backup run succeeded.'
		echo '# TYPE nebula_pgbackrest_backup_last_run_success gauge'
		echo "nebula_pgbackrest_backup_last_run_success{type=\"$2\"} $1"
		echo '# TYPE nebula_pgbackrest_backup_last_run_timestamp_seconds gauge'
		echo "nebula_pgbackrest_backup_last_run_timestamp_seconds{type=\"$2\"} $(date +%s)"
		echo '# TYPE nebula_pgbackrest_backup_last_run_duration_seconds gauge'
		echo "nebula_pgbackrest_backup_last_run_duration_seconds{type=\"$2\"} $3"
		echo '# HELP nebula_pgbackrest_last_backup_timestamp_seconds Stop time of the newest backup in the repository, by type.'
		echo '# TYPE nebula_pgbackrest_last_backup_timestamp_seconds gauge'
		jq -r '.[0].backup // [] | group_by(.type)[] | max_by(.timestamp.stop)
			| "nebula_pgbackrest_last_backup_timestamp_seconds{type=\"\(.type)\"} \(.timestamp.stop)"' <<<"$info"
		echo '# TYPE nebula_pgbackrest_last_backup_size_bytes gauge'
		jq -r '.[0].backup // [] | max_by(.timestamp.stop) // empty
			| "nebula_pgbackrest_last_backup_size_bytes \(.info.size)\nnebula_pgbackrest_last_backup_repo_size_bytes \(.info.repository.delta)"' <<<"$info"
		echo '# HELP nebula_pgbackrest_backups Number of backups kept in the repository.'
		echo '# TYPE nebula_pgbackrest_backups gauge'
		jq -r '"nebula_pgbackrest_backups \(.[0].backup // [] | length)"' <<<"$info"
		echo '# HELP nebula_pgbackrest_stanza_status 0 = ok (pgBackRest status code).'
		echo '# TYPE nebula_pgbackrest_stanza_status gauge'
		jq -r '"nebula_pgbackrest_stanza_status \(.[0].status.code // 99)"' <<<"$info"
	} | write_metrics pgbackrest
}

case $cmd in
init)
	((lock)) && take_lock
	pgbr stanza-create
	pgbr check
	log "stanza ok; take the first full backup now: backup.sh full"
	;;
check) pgbr check ;;
info) pgbr info ;;
full | diff | incr)
	((lock)) && take_lock 3600
	start=$(date +%s)
	log "pgbackrest backup --type=$cmd"
	if pgbr backup --type="$cmd"; then
		emit_metrics 1 "$cmd" $(($(date +%s) - start))
		pgbr info | tail -n 6 >&2
		log "backup $cmd ok"
	else
		emit_metrics 0 "$cmd" $(($(date +%s) - start))
		die "backup $cmd failed"
	fi
	;;
*) die "usage: backup.sh init|check|info|full|diff|incr [--no-lock]" ;;
esac
