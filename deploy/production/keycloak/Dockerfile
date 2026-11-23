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

# --- guava override (CVE-2026-102554, guava deserialization DoS; ADR-0018) -------------------
# Keycloak 26.8.2 still bundles guava 33.6.0; the fix is in guava >= 33.7.2. We swap in 33.8.0
# (same failureaccess 1.0.3) before `kc.sh build`. The jar is verified by SHA-256 here; it was
# checked against Maven Central's SHA-1 and Guava's release PGP signature (key
# BDB5FA4FE719D787FB3D3197F6D4A1D411E9D1AE) when this pin was set.
# The file keeps its original name: Quarkus records classpath entries by path. Trivy reads the
# version from the jar's pom.properties, not from the name.
# Self-expiring: once Keycloak bundles a fixed guava, this build FAILS and asks for removal.
ADD --chown=1000:0 --checksum=sha256:97b79ec3f2c0a6313a0c2df51ce1abdf8907e6531803467b81efcaaf99b35161 \
    https://repo1.maven.org/maven2/com/google/guava/guava/33.8.0-jre/guava-33.8.0-jre.jar /tmp/guava.jar
RUN set -eu; \
    cd /opt/keycloak/lib/lib/main; \
    set -- com.google.guava.guava-*.jar; \
    [ "$#" -eq 1 ] && [ -f "$1" ] || { echo "expected exactly one bundled guava jar, found: $*" >&2; exit 1; }; \
    bundled="$1"; ver="${bundled#com.google.guava.guava-}"; ver="${ver%-jre.jar}"; fixed=33.7.2; \
    if [ "$(printf '%s\n%s\n' "$fixed" "$ver" | sort -V | head -n1)" = "$fixed" ]; then \
      echo "Keycloak now bundles guava $ver (>= $fixed): remove the guava override from this Dockerfile and the CVE-2026-102554 entry from .trivyignore.yaml" >&2; \
      exit 1; \
    fi; \
    cp /tmp/guava.jar "$bundled"; rm /tmp/guava.jar; \
    echo "guava $ver replaced by 33.8.0-jre in $bundled"
# ----------------------------------------------------------------------------------------------

RUN /opt/keycloak/bin/kc.sh build

FROM quay.io/keycloak/keycloak:26.8.2@sha256:7e66a96e4739239070aeaf72e4c5decf0f7f682cfc725b9f7ea2b336eaba38b3
COPY --from=builder /opt/keycloak/ /opt/keycloak/
COPY --chmod=0755 nebula-entrypoint.sh /opt/keycloak/bin/nebula-entrypoint.sh
USER 1000
ENTRYPOINT ["/opt/keycloak/bin/nebula-entrypoint.sh"]
CMD ["start", "--optimized"]
