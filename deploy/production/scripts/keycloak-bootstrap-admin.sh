#!/usr/bin/env bash
# Creates the FIRST, TEMPORARY Keycloak admin in the master realm. Prompts for the password: it is
# never stored in a file, env var or shell history. Afterwards create a personal admin with MFA in
# the admin console (SSH tunnel) and DELETE this temporary account (README "İlk admin").
#
#   sudo ./scripts/keycloak-bootstrap-admin.sh [username]
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

user=${1:-temp-admin}
compose up -d --wait postgres
# A second Keycloak process against the same database is fine for this one-shot command.
compose run --rm --no-deps -it keycloak bootstrap-admin user --username "$user" --optimized
log "temporary admin '$user' created. Open the console via: ssh -L ${KEYCLOAK_ADMIN_TUNNEL_PORT:-8081}:127.0.0.1:${KEYCLOAK_ADMIN_TUNNEL_PORT:-8081} <host>"
log "then http://localhost:${KEYCLOAK_ADMIN_TUNNEL_PORT:-8081}/admin/"
