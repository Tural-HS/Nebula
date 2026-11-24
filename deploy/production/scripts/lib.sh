#!/usr/bin/env bash
# Shared helpers for the production scripts. Source it; do not execute it.
# shellcheck disable=SC2034 # variables are used by the sourcing scripts

set -Eeuo pipefail
umask 027

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${NEBULA_ENV_FILE:-$DEPLOY_DIR/.env}"

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

[[ -r $ENV_FILE ]] || die "missing $ENV_FILE (copy .env.example)"

# Loads .env into the environment of this script (simple KEY=VALUE lines only).
load_env() {
	set -a
	# shellcheck disable=SC1090
	source "$ENV_FILE"
	set +a
	STATE_DIR="${NEBULA_STATE_DIR:-/var/lib/nebula}"
	SECRETS_DIR="${NEBULA_SECRETS_DIR:-/etc/nebula/secrets}"
	TEXTFILE_DIR="$STATE_DIR/textfile"
	PROJECT="${COMPOSE_PROJECT_NAME:-nebula-prod}"
	[[ $PROJECT != nebula ]] || die "COMPOSE_PROJECT_NAME=nebula is reserved for the dev stack"
}

# Compose files, in order: COMPOSE_FILE from .env if set (the same list a manual `docker compose` in
# this directory reads, e.g. compose.yaml:compose.shared-host.yaml), else compose.yaml; then the
# optional site-specific overrides in NEBULA_COMPOSE_OVERRIDE (e.g. different limits). Both are
# colon-separated; relative paths are relative to this directory.
compose_files() {
	local -a list extra
	local f
	IFS=: read -r -a list <<<"${COMPOSE_FILE:-compose.yaml}"
	IFS=: read -r -a extra <<<"${NEBULA_COMPOSE_OVERRIDE:-}"
	for f in "${list[@]}" "${extra[@]}"; do
		[[ -n $f ]] || continue
		[[ $f == /* ]] || f="$DEPLOY_DIR/$f"
		printf '%s\n' "$f"
	done
}

compose() {
	local -a files=()
	local f
	while IFS= read -r f; do files+=(-f "$f"); done < <(compose_files)
	docker compose --project-directory "$DEPLOY_DIR" --env-file "$ENV_FILE" "${files[@]}" "$@"
}

# Same-host pilot backup target (compose.local-s3.yaml, README "Pulsuz yerel backup (pilot)").
# On when the override is in the compose file list or NEBULA_LOCAL_S3=1 (e.g. before .env lists it).
LOCAL_S3_UID_DEFAULT=10002
local_s3_enabled() {
	[[ ${NEBULA_LOCAL_S3:-0} == 1 ]] || compose_files | grep -q '/compose\.local-s3\.yaml$'
}
local_s3_paths() { # sets LOCAL_S3_DATA, LOCAL_S3_TLS, LOCAL_S3_UID (after load_env)
	LOCAL_S3_DATA="$STATE_DIR/backup-s3"
	LOCAL_S3_TLS="$STATE_DIR/backup-s3-tls"
	LOCAL_S3_UID="${NEBULA_LOCAL_S3_UID:-$LOCAL_S3_UID_DEFAULT}"
}

# psql inside the postgres container as the superuser via the local socket (peer auth).
psql_prod() { # psql_prod DB SQL
	compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -At -d "$1" -c "$2"
}

# Applies postgres/initdb/20-nebula-runtime-role.sql (idempotent): runtime role nebula_app, its
# password from the secret file, CONNECT; removes the old monitoring function. Table privileges
# follow from `nebula migrate up` (NEBULA_DB_RUNTIME_ROLE).
ensure_db_roles() {
	compose exec -T postgres psql -X -q -v ON_ERROR_STOP=1 -d postgres -f - \
		<"$DEPLOY_DIR/postgres/initdb/20-nebula-runtime-role.sql"
}

# The deploy/production layout from ADR-0018 on needs a Nebula image that reads *_FILE secrets
# and has `nebula healthcheck`. Probe (image must be pulled): without network the healthcheck of
# a new binary fails to connect (exit 1); an older binary rejects the command (exit 2).
image_supports_file_secrets() { # image_supports_file_secrets IMAGE
	local rc=0
	docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
		--entrypoint /nebula "$1" healthcheck -timeout 1ms >/dev/null 2>&1 || rc=$?
	[[ $rc == 1 ]]
}

schema_version() { psql_prod nebula "SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied" 2>/dev/null || echo 0; }

# Exclusive lock: deploy, rollback, backup and restore never overlap.
take_lock() {
	mkdir -p "$STATE_DIR"
	exec 9>"$STATE_DIR/ops.lock"
	flock -w "${1:-600}" 9 || die "another operation holds $STATE_DIR/ops.lock"
}

# Atomically writes a Prometheus textfile-collector file read by Alloy.
write_metrics() { # write_metrics name <<<"metrics text"
	mkdir -p "$TEXTFILE_DIR"
	chmod 0755 "$TEXTFILE_DIR"
	local tmp
	tmp="$(mktemp "$TEXTFILE_DIR/.$1.XXXXXX")"
	cat >"$tmp"
	chmod 0644 "$tmp"
	mv -f "$tmp" "$TEXTFILE_DIR/$1.prom"
}

# Image references must be pinned: repo[:tag]@sha256:<64 hex>.
is_pinned() { [[ $1 =~ ^[a-z0-9._/:-]+@sha256:[a-f0-9]{64}$ ]]; }

confirm() { # confirm "question" -> requires typing "yes" (skipped with ASSUME_YES=1)
	[[ ${ASSUME_YES:-0} == 1 ]] && return 0
	local answer
	read -r -p "$1 Type 'yes' to continue: " answer
	[[ $answer == yes ]] || die "aborted"
}

IMAGE_KEYS=(NEBULA_IMAGE NEBULA_WEB_IMAGE POSTGRES_IMAGE KEYCLOAK_IMAGE)
# Request path (gates deploys) vs. auxiliary services (monitoring; failures only warn).
APP_SERVICES=(postgres keycloak nebula caddy)
AUX_SERVICES=(alloy)

# set_env KEY VALUE: rewrites (or appends) KEY=VALUE in .env, keeping its permissions.
set_env() {
	local key=$1 value=$2 tmp
	tmp="$(mktemp "$ENV_FILE.XXXXXX")"
	chmod --reference="$ENV_FILE" "$tmp" 2>/dev/null || chmod 0640 "$tmp"
	awk -v k="$key" -v v="$value" 'BEGIN{done=0} $0 ~ "^"k"=" {print k"="v; done=1; next} {print} END{if(!done) print k"="v}' "$ENV_FILE" >"$tmp"
	mv -f "$tmp" "$ENV_FILE"
}

# Release records: $STATE_DIR/releases/<id>.release (image refs + schema version), history.log.
record_release() { # record_release note
	local id file
	id="$(date -u +%Y%m%dT%H%M%SZ)"
	file="$STATE_DIR/releases/$id.release"
	mkdir -p "$STATE_DIR/releases"
	load_env
	{
		for k in "${IMAGE_KEYS[@]}"; do printf '%s=%s\n' "$k" "${!k}"; done
		printf 'SCHEMA_VERSION=%s\nDEPLOYED_AT=%s\nNOTE=%q\n' "$(schema_version)" "$id" "$1"
	} >"$file"
	printf '%s %s\n' "$id" "$1" >>"$STATE_DIR/releases/history.log"
	ln -sfn "$id.release" "$STATE_DIR/releases/current"
	log "recorded release $id ($1)"
}

# End-to-end check through Caddy on this host (real TLS, real routing), without public DNS.
smoke_test() {
	local host=$NEBULA_HOST auth=$NEBULA_AUTH_HOST port=${NEBULA_HTTPS_PORT:-443} addr=${SMOKE_ADDR:-127.0.0.1} code
	local -a curl_opts=(-sS -o /dev/null -w '%{http_code}' --max-time 10
		--resolve "$host:$port:$addr" --resolve "$auth:$port:$addr")
	[[ ${SMOKE_INSECURE:-0} == 1 ]] && curl_opts+=(-k) # local tests with an internal CA only
	code="$(curl "${curl_opts[@]}" "https://$host:$port/config.json")" || code=000
	[[ $code == 200 ]] || { log "smoke: /config.json -> $code"; return 1; }
	code="$(curl "${curl_opts[@]}" "https://$host:$port/")" || code=000
	[[ $code == 200 ]] || { log "smoke: / -> $code"; return 1; }
	# API reachable and enforcing auth (no token -> 401).
	code="$(curl "${curl_opts[@]}" "https://$host:$port/api/v1/me")" || code=000
	[[ $code == 401 ]] || { log "smoke: /api/v1/me -> $code (want 401)"; return 1; }
	code="$(curl "${curl_opts[@]}" "https://$auth:$port/realms/${KEYCLOAK_REALM:-nebula}/.well-known/openid-configuration")" || code=000
	[[ $code == 200 ]] || { log "smoke: OIDC discovery -> $code"; return 1; }
	code="$(curl "${curl_opts[@]}" "https://$auth:$port/admin/")" || code=000
	[[ $code == 404 ]] || { log "smoke: /admin/ is publicly reachable ($code)"; return 1; }
	log "smoke test ok"
}
