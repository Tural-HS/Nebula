#!/usr/bin/env bash
# Creates the initial secret files for a fresh installation. Idempotent: an existing file is
# never overwritten (rotation is a deliberate, documented procedure; see README "Sirlər").
#
#   sudo ./scripts/gen-secrets.sh            # uses NEBULA_SECRETS_DIR from .env
#
# Layout (dir 0750 root:nebula-secrets):
#   <name>        0440 root:nebula-secrets  mounted into containers as Docker Compose secrets
#   nebula.env    0600 root:root            read only by the compose CLI (env_file for Nebula)
# External credentials (S3 keys, remote-write password, SMTP) are created as placeholders that
# MUST be replaced before the first start; preflight in deploy.sh refuses to run with them.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

[[ $EUID -eq 0 || ${NEBULA_ALLOW_NONROOT:-0} == 1 ]] || die "run as root (sudo)"
command -v openssl >/dev/null || die "openssl is required"

GID="${NEBULA_SECRETS_GID:-10001}"
OWNER=root
if [[ $EUID -eq 0 ]]; then
	getent group "$GID" >/dev/null || groupadd --system --gid "$GID" nebula-secrets
else
	OWNER="$(id -un)" # local validation only (NEBULA_ALLOW_NONROOT=1)
fi

install -d -m 0750 -o "$OWNER" -g "$GID" "$SECRETS_DIR"
install -d -m 0750 -o "$OWNER" -g "$GID" "$STATE_DIR"

created=()
put() { # put name value [mode] [group]
	local file="$SECRETS_DIR/$1" mode=${3:-0440} group=${4:-$GID}
	if [[ -e $file ]]; then
		log "keep    $file"
		return
	fi
	local tmp
	tmp="$(mktemp "$SECRETS_DIR/.tmp.XXXXXX")"
	printf '%s' "$2" >"$tmp"
	chown "$OWNER:$group" "$tmp"
	chmod "$mode" "$tmp"
	mv "$tmp" "$file"
	created+=("$1")
	log "created $file"
}

# URL-safe random passwords (hex, 192 bits): they are embedded in a postgres:// URL.
pw() { openssl rand -hex 24; }

NEBULA_DB_PW="$(cat "$SECRETS_DIR/nebula_db_password" 2>/dev/null || pw)"

put postgres_superuser_password "$(pw)"
put nebula_db_password "$NEBULA_DB_PW"
put keycloak_db_password "$(pw)"
put monitoring_db_password "$(pw)"
# pgBackRest repository encryption passphrase. LOSING IT MAKES EVERY BACKUP UNREADABLE:
# store a copy offline / in OpenBao before the first backup.
put pgbackrest_cipher_pass "$(openssl rand -base64 48 | tr -d '\n')"
put pgbackrest_s3_key "REPLACE_ME"
put pgbackrest_s3_key_secret "REPLACE_ME"
put remote_write_password "REPLACE_ME"

put nebula.env "$(cat <<ENV
# Nebula runtime secrets (env_file). 0600 root:root. Never commit, never paste into tickets.
NEBULA_DATABASE_URL=postgres://nebula:${NEBULA_DB_PW}@postgres:5432/nebula?sslmode=disable
NEBULA_ENCRYPTION_KEY=$(openssl rand -base64 32)
# Retired keys during rotation (comma separated), see README / nebula rotate-secrets.
NEBULA_ENCRYPTION_KEYS_OLD=
# Outgoing mail (STARTTLS mandatory in production). Empty host disables email.
NEBULA_SMTP_HOST=
NEBULA_SMTP_PORT=587
NEBULA_SMTP_USERNAME=
NEBULA_SMTP_PASSWORD=
NEBULA_SMTP_FROM=Nebula <noreply@example.com>
NEBULA_SMTP_TLS=starttls
ENV
)" 0600 0

if ((${#created[@]})); then
	log "created: ${created[*]}"
	log "NEXT: replace REPLACE_ME in pgbackrest_s3_key, pgbackrest_s3_key_secret, remote_write_password;"
	log "      fill SMTP in nebula.env; back up pgbackrest_cipher_pass and NEBULA_ENCRYPTION_KEY offline."
fi
