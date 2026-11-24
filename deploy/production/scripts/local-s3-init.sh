#!/usr/bin/env bash
# One-time setup of the same-host pilot backup target (compose.local-s3.yaml, README "Pulsuz yerel
# backup (pilot)"). Idempotent: existing, valid material is kept.
#
#   sudo ./scripts/local-s3-init.sh            # TLS material + bucket directory
#   sudo ./scripts/local-s3-init.sh --renew    # new CA and certificate (then: docker compose up -d --force-recreate backup-s3 postgres)
#
# Creates, owned by the backup-s3 uid (NEBULA_LOCAL_S3_UID, default 10002):
#   $NEBULA_STATE_DIR/backup-s3/                 0700  posix backend root (one directory per bucket)
#   $NEBULA_STATE_DIR/backup-s3/<bucket>/        0700  PGBACKREST_S3_BUCKET
#   $NEBULA_STATE_DIR/backup-s3-tls/             0755  root
#     ca.crt   0644 root   self-signed CA; pgBackRest verifies the server against it
#     tls.crt  0644 root   server certificate, SAN DNS:backup-s3, signed by the CA
#     tls.key  0400 uid    server key, readable only by backup-s3
# The CA private key is used once to sign and then deleted: nothing else can ever be signed with it,
# and --renew replaces CA and certificate together.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
local_s3_paths

renew=0
case ${1:-} in
"") ;;
--renew) renew=1 ;;
*) die "usage: local-s3-init.sh [--renew]" ;;
esac

[[ $EUID -eq 0 || ${NEBULA_ALLOW_NONROOT:-0} == 1 ]] || die "run as root (sudo)"
command -v openssl >/dev/null || die "openssl is required"
bucket=${PGBACKREST_S3_BUCKET:?set PGBACKREST_S3_BUCKET in .env}
[[ $bucket =~ ^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$ ]] || die "PGBACKREST_S3_BUCKET '$bucket' is not a valid bucket name"
[[ $LOCAL_S3_UID =~ ^[0-9]+$ && $LOCAL_S3_UID -ge 1000 ]] || die "NEBULA_LOCAL_S3_UID must be a numeric uid >= 1000"
DAYS=${NEBULA_LOCAL_S3_CERT_DAYS:-3650} # internal-only name; preflight warns 30 days before expiry

# --- Bucket directory (data stays on --renew) ---
[[ -d $STATE_DIR ]] || install -d -m 0750 "$STATE_DIR" # gen-secrets.sh sets its group
install -d -m 0700 -o "$LOCAL_S3_UID" -g "$LOCAL_S3_UID" "$LOCAL_S3_DATA" "$LOCAL_S3_DATA/$bucket"
log "bucket  $LOCAL_S3_DATA/$bucket"

# --- TLS ---
install -d -m 0755 -o root -g root "$LOCAL_S3_TLS"
valid() {
	[[ -s $LOCAL_S3_TLS/ca.crt && -s $LOCAL_S3_TLS/tls.crt && -s $LOCAL_S3_TLS/tls.key ]] || return 1
	openssl verify -CAfile "$LOCAL_S3_TLS/ca.crt" "$LOCAL_S3_TLS/tls.crt" >/dev/null 2>&1 || return 1
	openssl x509 -noout -checkhost backup-s3 -in "$LOCAL_S3_TLS/tls.crt" | grep -q 'does match' || return 1
	openssl x509 -noout -checkend $((30 * 86400)) -in "$LOCAL_S3_TLS/tls.crt" >/dev/null || return 1
	# The key belongs to the certificate.
	[[ "$(openssl x509 -noout -pubkey -in "$LOCAL_S3_TLS/tls.crt")" == "$(openssl pkey -pubout -in "$LOCAL_S3_TLS/tls.key")" ]]
}

if ((renew == 0)) && valid; then
	log "keep    $LOCAL_S3_TLS (valid until $(openssl x509 -noout -enddate -in "$LOCAL_S3_TLS/tls.crt" | cut -d= -f2))"
else
	work="$(mktemp -d "$LOCAL_S3_TLS/.new.XXXXXX")"
	trap 'rm -rf "$work"' EXIT
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days "$DAYS" \
		-subj "/CN=Nebula backup-s3 local CA" \
		-addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
		-addext "keyUsage=critical,keyCertSign,cRLSign" \
		-keyout "$work/ca.key" -out "$work/ca.crt" 2>/dev/null
	openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-subj "/CN=backup-s3" -keyout "$work/tls.key" -out "$work/tls.csr" 2>/dev/null
	openssl x509 -req -in "$work/tls.csr" -CA "$work/ca.crt" -CAkey "$work/ca.key" -CAcreateserial \
		-days "$DAYS" -out "$work/tls.crt" -extfile <(printf '%s\n' \
			"basicConstraints=critical,CA:FALSE" \
			"keyUsage=critical,digitalSignature" \
			"extendedKeyUsage=serverAuth" \
			"subjectAltName=DNS:backup-s3") 2>/dev/null
	rm -f "$work/ca.key" "$work/tls.csr" "$work/ca.srl"
	chown root:root "$work/ca.crt" "$work/tls.crt"
	chmod 0644 "$work/ca.crt" "$work/tls.crt"
	chown "$LOCAL_S3_UID:$LOCAL_S3_UID" "$work/tls.key"
	chmod 0400 "$work/tls.key"
	# Key first, CA last: a reader never sees a CA that does not match the certificate for long.
	mv -f "$work/tls.key" "$LOCAL_S3_TLS/tls.key"
	mv -f "$work/tls.crt" "$LOCAL_S3_TLS/tls.crt"
	mv -f "$work/ca.crt" "$LOCAL_S3_TLS/ca.crt"
	valid || die "generated TLS material does not verify"
	log "created $LOCAL_S3_TLS (CA + certificate for backup-s3, $DAYS days)"
	if ((renew)); then
		log "NEXT: docker compose up -d --force-recreate backup-s3 postgres  (both read the files at start)"
	fi
fi

log "local S3 ready: NEBULA_LOCAL_S3=1 ./scripts/gen-secrets.sh (keys), then ./scripts/preflight.sh"
