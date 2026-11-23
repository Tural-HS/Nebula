SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DEV_DB  := postgres://nebula:nebula-dev-only@localhost:5432/nebula?sslmode=disable
TEST_DB := postgres://nebula:nebula-dev-only@localhost:5432/nebula_test?sslmode=disable
OIDC_ISSUER   := http://localhost:8081/realms/nebula
OIDC_AUDIENCE := nebula-api
OIDC_JWKS_URL := $(OIDC_ISSUER)/protocol/openid-connect/certs
OIDC_CLIENT   := nebula-cli
# Dev-only seed users from deploy/keycloak/nebula-realm.json.
DEV_PASSWORD_alice := alice-dev-only
DEV_PASSWORD_bob   := bob-dev-only
# $(USER) is also the shell login name, so only honour it when given on the command line.
TOKEN_USER := $(if $(filter command line,$(origin USER)),$(USER),alice)
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.10.0

.PHONY: help
help: ## Show targets
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-18s %s\n",$$1,$$2}'

.PHONY: build
build: ## Build the binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/nebula ./cmd/nebula

.PHONY: test
test: ## Unit tests with race detector
	go test -race -count=1 ./...

.PHONY: test-integration
test-integration: db-up ## Unit + integration tests against local Postgres
	NEBULA_TEST_DATABASE_URL='$(TEST_DB)' go test -race -count=1 -coverpkg=./internal/... -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

.PHONY: lint
lint: ## Static analysis
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

.PHONY: db-up
db-up: ## Start local Postgres and wait until healthy
	docker compose up -d --wait postgres

.PHONY: idp-up
idp-up: ## Start local Keycloak (realm nebula) and wait until healthy
	docker compose up -d --wait keycloak

.PHONY: token
token: ## Print a dev access token (USER=alice|bob, default alice)
	@set -o pipefail; pw='$(DEV_PASSWORD_$(TOKEN_USER))'; \
	if [[ -z "$$pw" ]]; then echo "unknown dev user '$(TOKEN_USER)' (use alice or bob)" >&2; exit 1; fi; \
	curl -fsS --max-time 10 '$(OIDC_ISSUER)/protocol/openid-connect/token' \
		--data-urlencode grant_type=password \
		--data-urlencode client_id='$(OIDC_CLIENT)' \
		--data-urlencode username='$(TOKEN_USER)' \
		--data-urlencode "password=$$pw" \
		--data-urlencode scope=openid \
	| jq -er .access_token

.PHONY: migrate
migrate: db-up ## Apply migrations to the local dev database
	NEBULA_DATABASE_URL='$(DEV_DB)' go run ./cmd/nebula migrate up

.PHONY: run
run: migrate idp-up ## Run the API locally (Postgres + Keycloak in Docker)
	NEBULA_ENV=development NEBULA_LOG_FORMAT=text NEBULA_DATABASE_URL='$(DEV_DB)' \
	NEBULA_OIDC_ISSUER='$(OIDC_ISSUER)' NEBULA_OIDC_AUDIENCE='$(OIDC_AUDIENCE)' \
	NEBULA_OIDC_JWKS_URL='$(OIDC_JWKS_URL)' \
	go run ./cmd/nebula serve

.PHONY: up
up: ## Run the full stack in Docker
	docker compose up -d --build --wait

.PHONY: down
down: ## Stop the stack (keeps data)
	docker compose down

.PHONY: check
check: lint test-integration vuln ## Everything CI runs
