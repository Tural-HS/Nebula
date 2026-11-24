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

# Required secrets must be non-empty; the optional ones exist (Compose mounts them) but may be empty.
required=(postgres_superuser_password nebula_db_password nebula_app_db_password keycloak_db_password
	monitoring_db_password pgbackrest_cipher_pass pgbackrest_s3_key pgbackrest_s3_key_secret
	remote_write_password nebula_encryption_key nebula_evidence_signing_key)
optional=(nebula_encryption_keys_old nebula_smtp_password)
for s in "${required[@]}" "${optional[@]}"; do
	f="$SECRETS_DIR/$s"
	if [[ ! -f $f ]]; then bad "missing secret $f (run scripts/gen-secrets.sh)"; continue; fi
	if [[ ! -s $f && " ${optional[*]} " != *" $s "* ]]; then bad "secret $f is empty"; continue; fi
	grep -q REPLACE_ME "$f" && bad "$f still contains REPLACE_ME"
	mode="$(stat -c %a "$f" 2>/dev/null || stat -f %Lp "$f")"
	[[ $mode == 440 || $mode == 400 ]] || bad "$f must be mode 0440 (is $mode)"
done
# 32 random bytes, base64 (Nebula refuses anything else; checked here without printing it).
for k in nebula_encryption_key nebula_evidence_signing_key; do
	[[ -r $SECRETS_DIR/$k ]] || continue
	n="$(tr -d '\r\n' <"$SECRETS_DIR/$k" | base64 -d 2>/dev/null | wc -c | tr -d ' ')" || n=0
	[[ $n == 32 ]] || bad "$k must be 32 bytes, base64 encoded (openssl rand -base64 32)"
done
if [[ -e $SECRETS_DIR/nebula.env ]]; then
	log "WARN: $SECRETS_DIR/nebula.env is obsolete (ADR-0018): move SMTP settings to .env and delete it"
fi
[[ ${NEBULA_SMTP_TLS:-starttls} =~ ^(starttls|tls)$ ]] || bad "NEBULA_SMTP_TLS must be starttls or tls in production"

compose config --quiet || bad "docker compose config failed"

((fail == 0)) || die "preflight failed"
log "preflight ok"
