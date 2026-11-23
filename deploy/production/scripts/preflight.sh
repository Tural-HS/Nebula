#!/usr/bin/env bash
# Fails fast on configuration that would break or weaken production. Run by deploy.sh; safe to
# run any time:  sudo ./scripts/preflight.sh
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

fail=0
bad() { log "FAIL: $*"; fail=1; }

for v in NEBULA_IMAGE NEBULA_WEB_IMAGE POSTGRES_IMAGE KEYCLOAK_IMAGE; do
	is_pinned "${!v:-}" || bad "$v must be name@sha256:<digest>, got '${!v:-}'"
	[[ ${!v:-} != *@sha256:0000000000000000000000000000000000000000000000000000000000000000 ]] || bad "$v still has the placeholder digest"
done
for v in NEBULA_HOST NEBULA_AUTH_HOST; do
	[[ ${!v:-} =~ ^[a-z0-9.-]+$ ]] || bad "$v is not a hostname"
	[[ ${!v:-} != *example.com ]] || log "WARN: $v is still an example.com placeholder"
done
[[ $NEBULA_HOST != "$NEBULA_AUTH_HOST" ]] || bad "NEBULA_HOST and NEBULA_AUTH_HOST must differ"

for s in postgres_superuser_password nebula_db_password keycloak_db_password monitoring_db_password \
	pgbackrest_cipher_pass pgbackrest_s3_key pgbackrest_s3_key_secret remote_write_password nebula.env; do
	f="$SECRETS_DIR/$s"
	if [[ ! -s $f ]]; then bad "missing secret $f (run scripts/gen-secrets.sh)"; continue; fi
	grep -q REPLACE_ME "$f" && bad "$f still contains REPLACE_ME"
	mode="$(stat -c %a "$f" 2>/dev/null || stat -f %Lp "$f")"
	case $s in
	nebula.env) [[ $mode == 600 ]] || bad "$f must be mode 0600 (is $mode)" ;;
	*) [[ $mode == 440 || $mode == 400 ]] || bad "$f must be mode 0440 (is $mode)" ;;
	esac
done

if [[ -r $SECRETS_DIR/nebula.env ]] && grep -Eq '^NEBULA_(ENV|WEBHOOK_ALLOW_PRIVATE|SMTP_TLS)=(development|true|none)$' "$SECRETS_DIR/nebula.env"; then
	bad "nebula.env weakens production (NEBULA_ENV/WEBHOOK_ALLOW_PRIVATE/SMTP_TLS)"
fi

compose config --quiet || bad "docker compose config failed"

((fail == 0)) || die "preflight failed"
log "preflight ok"
