#!/usr/bin/env bash
# Deploys new image digests: preflight -> pull -> pre-migration backup -> migrate -> switch ->
# health + smoke test -> record. On a failed switch the previous images are restored automatically.
#
#   sudo ./scripts/deploy.sh NEBULA_IMAGE=ghcr.io/tural-hs/nebula:v1.4.0@sha256:... \
#                            NEBULA_WEB_IMAGE=ghcr.io/tural-hs/nebula-web:v1.4.0@sha256:...
# Options: --skip-backup (no pre-migration backup), --no-auto-rollback, --verify (cosign)
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

skip_backup=0 auto_rollback=1 verify=${COSIGN_VERIFY:-0}
declare -A new=()
for arg in "$@"; do
	case $arg in
	--skip-backup) skip_backup=1 ;;
	--no-auto-rollback) auto_rollback=0 ;;
	--verify) verify=1 ;;
	*=*)
		key=${arg%%=*} val=${arg#*=}
		[[ " ${IMAGE_KEYS[*]} " == *" $key "* ]] || die "unknown key $key (allowed: ${IMAGE_KEYS[*]})"
		is_pinned "$val" || die "$key must be pinned by digest: name@sha256:<64 hex>"
		new[$key]=$val
		;;
	*) die "usage: deploy.sh KEY=image@sha256:... [--skip-backup] [--no-auto-rollback] [--verify]" ;;
	esac
done
((${#new[@]})) || die "nothing to deploy"

take_lock
ts="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$STATE_DIR/releases"
# First deploy with this tooling: keep what runs now as the rollback target.
if [[ ! -e $STATE_DIR/releases/current ]] && compose ps --status running -q nebula | grep -q .; then
	record_release "baseline before $ts"
fi

cp -p "$ENV_FILE" "$STATE_DIR/releases/env.$ts.bak"
for key in "${!new[@]}"; do
	log "$key: ${!key} -> ${new[$key]}"
	set_env "$key" "${new[$key]}"
done
load_env

restore_env() { cp -p "$STATE_DIR/releases/env.$ts.bak" "$ENV_FILE"; load_env; }

"$DEPLOY_DIR/scripts/preflight.sh" || { restore_env; die "preflight failed; .env restored"; }

if ((verify)); then
	command -v cosign >/dev/null || { restore_env; die "--verify needs cosign"; }
	for key in "${!new[@]}"; do
		cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com \
			--certificate-identity-regexp "^https://github.com/${COSIGN_REPO:-Tural-HS/Nebula}/.github/workflows/release.yml@refs/tags/v.*$" \
			"${new[$key]}" >/dev/null || { restore_env; die "signature verification failed for ${new[$key]}"; }
		log "signature ok: ${new[$key]}"
	done
fi

log "pulling images"
compose pull --quiet --ignore-buildable || { restore_env; die "pull failed; .env restored"; }

# Restore point BEFORE anything changes (including a new Postgres image). On a fresh install there
# is nothing to back up yet: run `backup.sh init` + `backup.sh full` first (README).
if ((skip_backup == 0)); then
	if compose ps --status running -q postgres | grep -q .; then
		log "pre-deploy backup (incremental)"
		"$DEPLOY_DIR/scripts/backup.sh" incr --no-lock || { restore_env; die "pre-deploy backup failed; nothing changed (use --skip-backup to override)"; }
	else
		log "WARN: postgres is not running: no pre-deploy backup"
	fi
fi

compose up -d --wait postgres
old_schema="$(schema_version)"

log "running migrations (schema $old_schema)"
if ! compose run --rm --no-deps migrate; then
	restore_env
	die "migration failed: images unchanged. Each goose migration runs in a transaction unless marked otherwise; check 'nebula migrate status' and the logs."
fi
new_schema="$(schema_version)"
log "schema $old_schema -> $new_schema"

# Gate on the request path only: a broken monitoring agent must not roll back a good release.
log "switching services"
if compose up -d --wait --wait-timeout 300 "${APP_SERVICES[@]}" && smoke_test; then
	compose up -d --remove-orphans
	compose up -d --wait --wait-timeout 120 "${AUX_SERVICES[@]}" || log "WARN: monitoring services are unhealthy (release kept): compose ps / compose logs alloy"
	"$DEPLOY_DIR/scripts/container-info.sh" || true
	record_release "deploy $ts"
	log "deploy ok"
	exit 0
fi

log "new release is unhealthy"
compose ps
compose logs --no-color --tail 100 nebula caddy keycloak >&2 || true
if ((auto_rollback)); then
	log "auto-rollback to the previous images (schema stays at $new_schema: migrations are expand/contract, the previous release must work with it)"
	restore_env
	compose up -d --wait --wait-timeout 300 "${APP_SERVICES[@]}" && smoke_test && die "deploy failed, previous images restored"
	die "deploy failed AND the previous images are unhealthy: follow the incident checklist in README"
fi
die "deploy failed; auto-rollback disabled"
