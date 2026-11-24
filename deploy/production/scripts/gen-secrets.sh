#!/usr/bin/env bash
# Creates the initial secret files for a fresh installation. Idempotent: an existing file is
# never overwritten (rotation is a deliberate, documented procedure; see README "Sirlər").
#
#   sudo ./scripts/gen-secrets.sh            # uses NEBULA_SECRETS_DIR from .env
#
# Layout (dir 0750 root:nebula-secrets): every secret is one file, 0440 root:nebula-secrets,
# mounted into the containers that need it as a Docker Compose secret (Nebula reads *_FILE).
# External credentials (S3 keys, remote-write password) are created as placeholders that MUST be
# replaced before the first start; preflight in deploy.sh refuses to run with them.
#
# Upgrade from the env_file layout (ADR-0018): an existing nebula.env is read once to carry over
# NEBULA_ENCRYPTION_KEY, NEBULA_ENCRYPTION_KEYS_OLD and NEBULA_SMTP_PASSWORD (never regenerated:
# a new encryption key would make stored webhook secrets unreadable).
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

# legacy KEY: value of KEY in a pre-ADR-0018 nebula.env (empty if absent).
legacy() {
	[[ -r $SECRETS_DIR/nebula.env ]] || return 0
	sed -n "s/^$1=//p" "$SECRETS_DIR/nebula.env" | tail -n 1
}

ENC_KEY="$(legacy NEBULA_ENCRYPTION_KEY)"
[[ -n $ENC_KEY ]] || ENC_KEY="$(openssl rand -base64 32)"

put postgres_superuser_password "$(pw)"
put nebula_db_password "$(pw)"      # schema owner: nebula migrate only
put nebula_app_db_password "$(pw)"  # runtime role nebula_app: DML only
put keycloak_db_password "$(pw)"
put monitoring_db_password "$(pw)"
# pgBackRest repository encryption passphrase. LOSING IT MAKES EVERY BACKUP UNREADABLE:
# store a copy offline / in OpenBao before the first backup.
put pgbackrest_cipher_pass "$(openssl rand -base64 48 | tr -d '\n')"
put pgbackrest_s3_key "REPLACE_ME"
put pgbackrest_s3_key_secret "REPLACE_ME"
put remote_write_password "REPLACE_ME"
# Nebula: webhook-secret encryption key (base64, 32 bytes). LOSING IT makes stored webhook secrets
# unreadable: keep an offline copy. Retired keys (comma separated) only during rotation.
put nebula_encryption_key "$ENC_KEY"
put nebula_encryption_keys_old "$(legacy NEBULA_ENCRYPTION_KEYS_OLD)"
# Audit evidence signing key (ADR-0029): an Ed25519 seed, base64 32 bytes. Its fingerprint is in
# Settings → Audit log and in every package. Losing it does not break old packages (they carry the
# public key), but auditors who pinned the fingerprint must be told about a new one.
put nebula_evidence_signing_key "$(openssl rand -base64 32)"
# SMTP password; empty = no SMTP authentication. Host, user and sender are in .env.
put nebula_smtp_password "$(legacy NEBULA_SMTP_PASSWORD)"

if ((${#created[@]})); then
	log "created: ${created[*]}"
	log "NEXT: replace REPLACE_ME in pgbackrest_s3_key, pgbackrest_s3_key_secret, remote_write_password;"
	log "      set NEBULA_SMTP_* in .env (password: nebula_smtp_password);"
	log "      back up pgbackrest_cipher_pass, nebula_encryption_key and nebula_evidence_signing_key offline."
fi
if [[ -e $SECRETS_DIR/nebula.env ]]; then
	log "NOTE: $SECRETS_DIR/nebula.env is no longer used. Its secrets were carried over (if missing);"
	log "      copy NEBULA_SMTP_HOST/PORT/USERNAME/FROM/TLS into .env, then delete it (README §2.2)."
fi
