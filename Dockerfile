# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/nebula ./cmd/nebula
# Empty attachment directory owned by nonroot: a named volume mounted here
# inherits the ownership on first use (ADR-0022).
RUN mkdir -p /out/attachments

# Distroless: no shell, no package manager, runs as UID 65532.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
COPY --from=build /out/nebula /nebula
COPY --from=build --chown=65532:65532 --chmod=0700 /out/attachments /var/lib/nebula/attachments
USER nonroot:nonroot
EXPOSE 8080 9090
# No shell or curl in the image: the binary probes its own /readyz. Worker-only containers
# override it with ["CMD", "/nebula", "healthcheck", "-admin"]; one-shot "migrate" ignores it.
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 CMD ["/nebula", "healthcheck"]
ENTRYPOINT ["/nebula"]
CMD ["serve"]
