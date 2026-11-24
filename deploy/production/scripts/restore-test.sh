#!/usr/bin/env bash
# Monthly restore test: restores the newest backup + archived WAL from S3 into a THROWAWAY container
# and volume, then verifies it. Production containers, volumes and networks are never touched.
#
#   sudo ./scripts/restore-test.sh            # scheduled by systemd/nebula-restore-test.timer
#
# Checks: recovery completes and promotes; schema version equals production; core tables are
# non-empty when production's are; row counts within tolerance; Keycloak realm present.
# Records the measured duration (actual RTO for the database) and data lag (actual RPO).
#
#   RESTORE_TEST_REPO=2 sudo -E ./scripts/restore-test.sh   # the second repository (compose.repo2-offsite.yaml)
# The test container only ever sees one repository: repo2's settings and key files are mapped to
# repo1 inside it (same stanza and cipher passphrase).
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

name="nebula-restoretest-$(date -u +%Y%m%d%H%M%S)"
vol="$name-data"
stanza=${PGBACKREST_STANZA:-nebula}
start=$(date +%s)
tolerance_pct=${RESTORE_TEST_TOLERANCE_PCT:-2}

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker volume rm -f "$vol" >/dev/null 2>&1 || true
}
fail() {
	{
		echo '# TYPE nebula_restore_test_last_run_success gauge'
		echo 'nebula_restore_test_last_run_success 0'
		echo '# TYPE nebula_restore_test_last_run_timestamp_seconds gauge'
		echo "nebula_restore_test_last_run_timestamp_seconds $(date +%s)"
		# if, not &&: a false test as the group's last command would fail the pipeline (pipefail)
		# and exit via errexit before die logs the reason.
		if [[ -n ${last_ok:-} ]]; then echo "nebula_restore_test_last_success_timestamp_seconds $last_ok"; fi
	} | write_metrics restore_test
	die "restore test failed: $*"
}
trap cleanup EXIT
last_ok="$(sed -n 's/^nebula_restore_test_last_success_timestamp_seconds //p' "$TEXTFILE_DIR/restore_test.prom" 2>/dev/null || true)"

# Production reference values (read-only queries).
tables=(workspaces users projects items comments events)
declare -A prod=()
for t in "${tables[@]}"; do prod[$t]="$(psql_prod nebula "SELECT count(*) FROM $t")"; done
prod_schema="$(schema_version)"
# Archive the current WAL segment (pgbackrest check switches and waits), so the restore includes every
# row counted above; otherwise up to archive_timeout of recent writes is missing and the
# row-count tolerance fails on small or busy databases.
compose exec -T postgres nebula-pgbackrest --stanza="$stanza" check >/dev/null 2>&1 || fail "WAL archiving check failed (backup.sh check)"
log "production: schema $prod_schema, $(for t in "${tables[@]}"; do printf '%s=%s ' "$t" "${prod[$t]}"; done)"

# Repository under test: its connection settings, key files and network.
case ${RESTORE_TEST_REPO:-1} in
1)
	s3_endpoint=$PGBACKREST_S3_ENDPOINT s3_region=$PGBACKREST_S3_REGION s3_bucket=$PGBACKREST_S3_BUCKET
	s3_uri_style=${PGBACKREST_S3_URI_STYLE:-path} s3_path=${PGBACKREST_REPO_PATH:-/nebula}
	s3_verify=${PGBACKREST_S3_VERIFY_TLS:-y} key_prefix=pgbackrest_s3 network=${RESTORE_TEST_NETWORK:-bridge}
	# Optional private CA for the repository's TLS certificate (local S3: local-s3-init.sh's ca.crt).
	ca_file=${PGBACKREST_S3_CA_FILE:-}
	if [[ -z $ca_file ]] && local_s3_enabled; then ca_file=$STATE_DIR/backup-s3-tls/ca.crt; fi
	;;
2)
	s3_endpoint=${PGBACKREST_REPO2_S3_ENDPOINT:?} s3_region=${PGBACKREST_REPO2_S3_REGION:?} s3_bucket=${PGBACKREST_REPO2_S3_BUCKET:?}
	s3_uri_style=${PGBACKREST_REPO2_S3_URI_STYLE:-host} s3_path=${PGBACKREST_REPO2_PATH:-/nebula-prod}
	s3_verify=${PGBACKREST_REPO2_S3_VERIFY_TLS:-y} key_prefix=pgbackrest_repo2_s3 network=${RESTORE_TEST_REPO2_NETWORK:-bridge}
	ca_file=${PGBACKREST_REPO2_S3_CA_FILE:-}
	;;
*) die "RESTORE_TEST_REPO must be 1 or 2" ;;
esac
log "repository ${RESTORE_TEST_REPO:-1}: $s3_endpoint/$s3_bucket$s3_path (network $network)"
ca_opts=()
if [[ -n $ca_file ]]; then
	[[ -r $ca_file ]] || fail "missing CA file $ca_file"
	ca_opts=(-v "$ca_file:/etc/pgbackrest/backup-s3-ca.crt:ro" -e PGBACKREST_REPO1_STORAGE_CA_FILE=/etc/pgbackrest/backup-s3-ca.crt)
fi

docker volume create "$vol" >/dev/null
# Same image as production. No production network: the default bridge only reaches S3
# (override with RESTORE_TEST_NETWORK when the repository is only reachable on another network, e.g.
# <project>-backup for the same-host backup-s3: internal, so the copy reaches nothing else).
# archive_mode=off and --archive-mode=off: the restored copy can never push WAL into the repository.
# --init: async archive-get workers are re-parented to PID 1, which must not be the postmaster.
docker run -d --init --name "$name" --user 999:999 --group-add "${NEBULA_SECRETS_GID:-10001}" \
	--read-only --cap-drop ALL --security-opt no-new-privileges:true \
	--memory 2g --cpus 2 --network "$network" \
	--tmpfs /var/run/postgresql:size=1m,mode=1777 --tmpfs /tmp:size=256m,mode=1777 \
	--tmpfs /var/spool/pgbackrest:size=1g,mode=1777 \
	-v "$vol:/var/lib/postgresql" \
	-v "$DEPLOY_DIR/postgres/postgresql.conf:/etc/postgresql/postgresql.conf:ro" \
	-v "$SECRETS_DIR/${key_prefix}_key:/run/secrets/pgbackrest_s3_key:ro" \
	-v "$SECRETS_DIR/${key_prefix}_key_secret:/run/secrets/pgbackrest_s3_key_secret:ro" \
	-v "$SECRETS_DIR/pgbackrest_cipher_pass:/run/secrets/pgbackrest_cipher_pass:ro" \
	-e PGBACKREST_REPO1_S3_ENDPOINT="$s3_endpoint" \
	-e PGBACKREST_REPO1_S3_REGION="$s3_region" \
	-e PGBACKREST_REPO1_S3_BUCKET="$s3_bucket" \
	-e PGBACKREST_REPO1_S3_URI_STYLE="$s3_uri_style" \
	-e PGBACKREST_REPO1_PATH="$s3_path" \
	-e PGBACKREST_REPO1_STORAGE_VERIFY_TLS="$s3_verify" \
	"${ca_opts[@]}" \
	--entrypoint bash \
	"$POSTGRES_IMAGE" -c '
		set -euo pipefail
		install -d -m 0700 /var/lib/postgresql/19/docker
		nebula-pgbackrest --stanza='"$stanza"' --archive-mode=off restore
		exec postgres -c config_file=/etc/postgresql/postgresql.conf -c archive_mode=off \
			-c listen_addresses= -c shared_buffers=256MB
	' >/dev/null

in_test() { docker exec "$name" psql -X -v ON_ERROR_STOP=1 -At -h /var/run/postgresql -U postgres "$@"; }

log "restoring into $name (waiting for recovery to finish)"
deadline=$((start + ${RESTORE_TEST_TIMEOUT:-7200}))
until [[ "$(in_test -d postgres -c 'SELECT pg_is_in_recovery()' 2>/dev/null)" == f ]]; do
	[[ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)" == true ]] || { docker logs --tail 50 "$name" >&2; fail "container exited"; }
	(($(date +%s) < deadline)) || fail "timeout"
	sleep 5
done
duration=$(($(date +%s) - start))

problems=()
[[ "$(in_test -d nebula -c 'SELECT coalesce(max(version_id),0) FROM goose_db_version WHERE is_applied')" == "$prod_schema" ]] ||
	problems+=("schema version differs")
for t in "${tables[@]}"; do
	r="$(in_test -d nebula -c "SELECT count(*) FROM $t")"
	p=${prod[$t]}
	log "  $t: restored=$r production=$p"
	if ((p > 0 && r == 0)); then problems+=("$t is empty"); fi
	if ((p > 0 && (p - r) * 100 > p * tolerance_pct)); then problems+=("$t differs by more than $tolerance_pct%"); fi
done
[[ "$(in_test -d keycloak -c "SELECT count(*) FROM realm WHERE name = '${KEYCLOAK_REALM:-nebula}'")" == 1 ]] ||
	problems+=("keycloak realm missing")
in_test -d nebula -c 'SELECT 1' >/dev/null || problems+=("health query failed")
lag="$(in_test -d nebula -c "SELECT coalesce(extract(epoch FROM now() - max(occurred_at))::bigint, 0) FROM events")"

((${#problems[@]} == 0)) || fail "${problems[*]}"

{
	echo '# HELP nebula_restore_test_last_success_timestamp_seconds Last successful restore test.'
	echo '# TYPE nebula_restore_test_last_success_timestamp_seconds gauge'
	echo "nebula_restore_test_last_success_timestamp_seconds $(date +%s)"
	echo '# TYPE nebula_restore_test_last_run_success gauge'
	echo 'nebula_restore_test_last_run_success 1'
	echo '# TYPE nebula_restore_test_last_run_timestamp_seconds gauge'
	echo "nebula_restore_test_last_run_timestamp_seconds $(date +%s)"
	echo '# HELP nebula_restore_test_duration_seconds Restore + WAL replay time (database RTO).'
	echo '# TYPE nebula_restore_test_duration_seconds gauge'
	echo "nebula_restore_test_duration_seconds $duration"
	echo '# HELP nebula_restore_test_data_age_seconds Age of the newest restored event at test time.'
	echo '# TYPE nebula_restore_test_data_age_seconds gauge'
	echo "nebula_restore_test_data_age_seconds $lag"
} | write_metrics restore_test
log "restore test ok in ${duration}s (newest restored event is ${lag}s old)"
