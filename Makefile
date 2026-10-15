.PHONY: run dev test test-db lint vuln check build docker db-up db-down migrate migrate-down migration generate

ENV = if [ -f .env ]; then set -a; . ./.env; set +a; fi;
COMPOSE = docker compose --env-file .env -f deploy/docker-compose.yml
TOOL = go tool -modfile=tools.mod
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

run:
	@$(ENV) go run ./cmd/api

dev:
	@$(ENV) $(TOOL) air

test:
	go test -race -count=1 ./...

test-db:
	@$(ENV) TEST_DATABASE_URL="$$DATABASE_URL" go test -race -count=1 ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	$(TOOL) staticcheck ./...

vuln:
	$(TOOL) govulncheck ./...

check: lint test vuln

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

docker:
	docker build --build-arg VERSION=$(VERSION) -t golang-server:$(VERSION) .

db-up:
	$(COMPOSE) up -d --wait --remove-orphans postgres
	$(COMPOSE) exec -T postgres /docker-entrypoint-initdb.d/10-supertokens.sh
	$(COMPOSE) up -d --wait supertokens

db-down:
	$(COMPOSE) down

migrate:
	@$(ENV) $(TOOL) goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@$(ENV) $(TOOL) goose -dir migrations postgres "$$DATABASE_URL" down

migration:
	@echo "$(name)" | grep -Eq '^[a-z0-9_]+$$' || (echo "usage: make migration name=create_businesses" && exit 1)
	$(TOOL) goose -dir migrations -s create $(name) sql

generate:
	@test -d queries || (echo "no queries/ yet: write the first query, then run make generate" && exit 1)
	$(TOOL) sqlc generate
