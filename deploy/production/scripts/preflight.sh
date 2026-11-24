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

[[ "$(compose_files | head -n 1)" == "$DEPLOY_DIR/compose.yaml" ]] || bad "COMPOSE_FILE must start with compose.yaml"

# Shared-host pilot (compose.shared-host.yaml, README "Paylaşılan hostda pilot").
if compose_files | grep -q '/compose\.shared-host\.yaml$'; then
	# Scripts and manual `docker compose` must see the same files: without the override a manual
	# `up` recreates Caddy on the outer proxy's ports and Nebula without its IdP route.
	[[ ${COMPOSE_FILE:-} == *compose.shared-host.yaml* ]] ||
		bad "shared host: set COMPOSE_FILE=compose.yaml:compose.shared-host.yaml in .env (not only NEBULA_COMPOSE_OVERRIDE)"
	net=${NEBULA_SHARED_PROXY_NETWORK:-} alias=${NEBULA_SHARED_PROXY_ALIAS:-nebula-edge}
	if [[ -z $net ]]; then
		bad "shared host: NEBULA_SHARED_PROXY_NETWORK is not set"
	elif ! subnets="$(docker network inspect -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}' "$net" 2>/dev/null)"; then
		bad "shared host: docker network '$net' does not exist (is the outer proxy stack running?)"
	else
		# Nebula's Caddy believes X-Forwarded-For from exactly this subnet.
		[[ " $subnets " == *" ${NEBULA_SHARED_PROXY_SUBNET:-unset} "* ]] ||
			bad "shared host: NEBULA_SHARED_PROXY_SUBNET=${NEBULA_SHARED_PROXY_SUBNET:-} is not the subnet of '$net' ($subnets)"
		# A second container answering to the alias would get a share of Nebula's traffic (DNS round robin).
		for id in $(docker network inspect -f '{{range $id, $c := .Containers}}{{$id}} {{end}}' "$net"); do
			[[ "$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$id")" != "$PROJECT" ]] || continue
			if docker inspect "$id" | jq -e --arg n "$net" --arg a "$alias" \
				'.[0] | (.Name | ltrimstr("/")) as $name | .NetworkSettings.Networks[$n] | ([$name] + (.Aliases // []) + (.DNSNames // [])) | index($a) != null' >/dev/null; then
				bad "shared host: another container on '$net' already answers to '$alias'"
			fi
		done
	fi
fi

compose config --quiet || bad "docker compose config failed"

((fail == 0)) || die "preflight failed"
log "preflight ok"
