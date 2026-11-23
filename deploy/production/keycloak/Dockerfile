# syntax=docker/dockerfile:1.7
# Keycloak with build-time options baked in, so the container runs `start --optimized` (no
# re-augmentation at boot, faster start, read-only /opt/keycloak).
# Build: docker build -t nebula-keycloak:<version> deploy/production/keycloak
FROM quay.io/keycloak/keycloak:26.8.2@sha256:7e66a96e4739239070aeaf72e4c5decf0f7f682cfc725b9f7ea2b336eaba38b3 AS builder
ENV KC_DB=postgres \
    KC_HEALTH_ENABLED=true \
    KC_METRICS_ENABLED=true \
    KC_CACHE=local
# Single node: local caches, no cluster discovery. Switch to KC_CACHE=ispn (jdbc-ping) for HA.
RUN /opt/keycloak/bin/kc.sh build

FROM quay.io/keycloak/keycloak:26.8.2@sha256:7e66a96e4739239070aeaf72e4c5decf0f7f682cfc725b9f7ea2b336eaba38b3
COPY --from=builder /opt/keycloak/ /opt/keycloak/
COPY --chmod=0755 nebula-entrypoint.sh /opt/keycloak/bin/nebula-entrypoint.sh
USER 1000
ENTRYPOINT ["/opt/keycloak/bin/nebula-entrypoint.sh"]
CMD ["start", "--optimized"]
