#!/usr/bin/env bash
# Reads the database password from a Docker secret file (Keycloak has no *_FILE convention for it)
# and hands over to kc.sh. Nothing secret is placed in the image or on the command line.
set -euo pipefail

if [[ -n ${NEBULA_KC_DB_PASSWORD_FILE:-} ]]; then
	KC_DB_PASSWORD="$(tr -d '\r\n' <"$NEBULA_KC_DB_PASSWORD_FILE")"
	export KC_DB_PASSWORD
fi
unset NEBULA_KC_DB_PASSWORD_FILE

exec /opt/keycloak/bin/kc.sh "$@"
