# syntax=docker/dockerfile:1.7
# Edge image: Caddy + the built Nebula web UI. Build from the repository root:
#   docker build -f deploy/production/web.Dockerfile -t nebula-web:<version> web
# (context is web/; web.Dockerfile.dockerignore keeps node_modules and test output out of it)

FROM node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS build
WORKDIR /web
COPY package.json package-lock.json ./
# --ignore-scripts: no dependency lifecycle scripts run during the build (supply-chain hardening).
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts --no-audit --no-fund
COPY . .
# public/config.json is the dev config; production serves /config.json from Caddy instead.
RUN npm run build && rm -f dist/config.json && find dist -name '*.map' -delete

FROM caddy:2.11.9-alpine@sha256:003a97808e781aa9f7e75cc2f43d34d461099031faf5e20faefb9f61126836f0
# The stock binary carries file capabilities (cap_net_bind_service+ep). Under no-new-privileges and
# cap_drop ALL such a binary cannot be executed, so strip them; port 80/443 binding comes from the
# namespaced sysctl net.ipv4.ip_unprivileged_port_start=0 in compose instead.
RUN apk add --no-cache --virtual .setcap libcap-setcap \
 && setcap -r /usr/bin/caddy \
 && apk del .setcap \
 && mkdir -p /data/caddy /config/caddy \
 && chown -R 65532:65532 /data /config
COPY --from=build /web/dist /srv
USER 65532:65532
