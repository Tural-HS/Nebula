#!/usr/bin/env bash
# Rolls the stack back to a previously deployed release (image digests recorded by deploy.sh).
#
#   sudo ./scripts/rollback.sh                       # previous release, images only
#   sudo ./scripts/rollback.sh --list
#   sudo ./scripts/rollback.sh --to 20261201T101500Z
#   sudo ./scripts/rollback.sh --migrate-down-to 9   # also revert migrations newer than 9 first
#
# Schema: migrations are expand/contract, so the previous release runs on the newer schema and
# an image-only rollback is the default. Reverting migrations runs their Down sections with the
# CURRENT image (the old one does not contain them). Some Down sections destroy data (see README);
# after a non-reversible migration the only way back is a point-in-time restore (restore.sh).
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

rel_dir="$STATE_DIR/releases"
target="" down_to=""
while (($#)); do
	case $1 in
	--list)
		cat "$rel_dir/history.log" 2>/dev/null || true
		exit 0
		;;
	--to) target=$2; shift ;;
	--migrate-down-to) down_to=$2; shift ;;
	*) die "unknown argument $1" ;;
	esac
	shift
done

[[ -e $rel_dir/current ]] || die "no release history in $rel_dir"
current="$(basename "$(readlink "$rel_dir/current")" .release)"
if [[ -z $target ]]; then
	target="$(awk -v c="$current" '$1 == c {print prev; exit} {prev = $1}' "$rel_dir/history.log")"
	[[ -n $target ]] || die "no release before $current"
fi
file="$rel_dir/$target.release"
[[ -r $file ]] || die "unknown release $target"

take_lock
target_schema="$(sed -n 's/^SCHEMA_VERSION=//p' "$file")"
cur_schema="$(schema_version)"
log "rollback $current -> $target (schema now $cur_schema, release was deployed at $target_schema)"
grep -E '^(NEBULA|POSTGRES|KEYCLOAK)_.*IMAGE=' "$file" >&2

if [[ -n $down_to ]]; then
	[[ $down_to =~ ^[0-9]+$ ]] || die "--migrate-down-to needs a version number"
	((down_to < cur_schema)) || die "schema is already at $cur_schema"
	confirm "Revert migrations $((down_to + 1))..$cur_schema with their Down sections (may DELETE data)?"
	"$DEPLOY_DIR/scripts/backup.sh" incr --no-lock
	while (($(schema_version) > down_to)); do
		log "migrate down from $(schema_version)"
		compose run --rm --no-deps migrate migrate down
	done
elif ((cur_schema > target_schema)); then
	log "WARN: schema $cur_schema is newer than the target release ($target_schema); keeping it (expand/contract)."
fi

confirm "Switch to release $target?"
cp -p "$ENV_FILE" "$rel_dir/env.rollback.$(date -u +%Y%m%dT%H%M%SZ).bak"
for k in "${IMAGE_KEYS[@]}"; do
	v="$(sed -n "s/^$k=//p" "$file")"
	[[ -n $v ]] && set_env "$k" "$v"
done
load_env
compose pull --quiet --ignore-buildable
compose up -d --wait --wait-timeout 300 "${APP_SERVICES[@]}"
compose up -d --remove-orphans
smoke_test || die "rolled-back release is unhealthy: follow the incident checklist"
record_release "rollback to $target"
