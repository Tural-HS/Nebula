# syntax=docker/dockerfile:1.7
# PostgreSQL 19 (Debian 13) + pgBackRest from the PGDG repository that the official image already
# configures. Build: docker build -t nebula-postgres:<version> deploy/production/postgres
FROM postgres:19.0-trixie@sha256:766ddae4967301b9802b341bb596905f11dd5c558149690a7a4d80c2f61bbcf9

ARG PGBACKREST_VERSION=2.60.1-1.pgdg13+1
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends "pgbackrest=${PGBACKREST_VERSION}"; \
    rm -rf /var/lib/apt/lists/*; \
    install -d -o postgres -g postgres -m 0750 /var/spool/pgbackrest; \
    pgbackrest version

COPY pgbackrest.conf /etc/pgbackrest/pgbackrest.conf
COPY --chmod=0755 nebula-pgbackrest /usr/local/bin/nebula-pgbackrest
COPY --chmod=0755 initdb/ /docker-entrypoint-initdb.d/

USER postgres
