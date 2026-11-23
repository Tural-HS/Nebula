#!/usr/bin/env bash
# Writes nebula_container_info{container_id, service, project} for the containers of this stack,
# so cAdvisor series (read from cgroups, without a Docker socket in any container) can be joined
# to compose service names:
#   container_memory_working_set_bytes * on (container_id) group_left (service) nebula_container_info
# Runs every minute (systemd/nebula-container-info.timer) and after each deploy.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

{
	echo '# HELP nebula_container_info Maps container ids to compose services (value is always 1).'
	echo '# TYPE nebula_container_info gauge'
	docker ps --no-trunc --filter "label=com.docker.compose.project=$PROJECT" \
		--format '{{.ID}} {{.Label "com.docker.compose.service"}}' |
		while read -r id service; do
			printf 'nebula_container_info{container_id="%s",service="%s",project="%s"} 1\n' "$id" "$service" "$PROJECT"
		done
} | write_metrics containers
