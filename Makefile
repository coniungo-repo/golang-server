.PHONY: run dev smoke test test-db admin-invite lint vuln check build docker db-up db-down migrate migrate-down migration generate deploy-staging deploy-production deploy-core-staging deploy-core-production

ENV = if [ -f .env ]; then set -a; . ./.env; set +a; fi;
COMPOSE = docker compose --env-file .env -f deploy/docker-compose.yml
TOOL = go tool -modfile=tools.mod
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

run:
	@$(ENV) go run ./cmd/api

dev:
	@$(ENV) $(TOOL) air

smoke:
	@./scripts/smoke-local.sh

test:
	go test -race -count=1 ./...

test-db:
	@$(ENV) TEST_DATABASE_URL="$$DATABASE_URL" TEST_SUPERTOKENS_URI="$$SUPERTOKENS_CONNECTION_URI" TEST_SUPERTOKENS_API_KEY="$$SUPERTOKENS_API_KEY" go test -race -count=1 ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	$(TOOL) staticcheck ./...

vuln:
	$(TOOL) govulncheck ./...

check: lint test vuln

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ ./cmd/...

docker:
	docker build --build-arg VERSION=$(VERSION) -t golang-server:$(VERSION) .

db-up:
	$(COMPOSE) up -d --wait --remove-orphans postgres
	$(COMPOSE) exec -T postgres /docker-entrypoint-initdb.d/10-supertokens.sh
	$(COMPOSE) up -d --wait supertokens

db-down:
	$(COMPOSE) down

migrate:
	@$(ENV) go run ./cmd/migrate

migrate-down:
	@$(ENV) $(TOOL) goose -dir migrations postgres "$$DATABASE_URL" down

migration:
	@echo "$(name)" | grep -Eq '^[a-z0-9_]+$$' || (echo "usage: make migration name=create_businesses" && exit 1)
	$(TOOL) goose -dir migrations -s create $(name) sql

admin-invite:
	@test -n "$(email)" || (echo "usage: make admin-invite email=you@example.com" && exit 1)
	@$(ENV) go run ./cmd/admin invite "$(email)"

generate:
	@test -d queries || (echo "no queries/ yet: write the first query, then run make generate" && exit 1)
	$(TOOL) sqlc generate

deploy-staging:
	fly deploy -c fly.staging.toml --build-arg VERSION=$(VERSION)

deploy-production:
	@test -z "$$(git status --porcelain)" || (echo "commit or stash changes before deploying to production" && exit 1)
	fly deploy -c fly.production.toml --build-arg VERSION=$(VERSION)

deploy-core-staging:
	fly deploy -c deploy/fly/supertokens.staging.toml

deploy-core-production:
	fly deploy -c deploy/fly/supertokens.production.toml
