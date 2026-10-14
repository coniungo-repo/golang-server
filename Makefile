.PHONY: run dev test lint db-up db-down migrate migrate-down migration generate

ENV = if [ -f .env ]; then set -a; . ./.env; set +a; fi;
COMPOSE = docker compose --env-file .env -f deploy/docker-compose.yml

run:
	@$(ENV) go run ./cmd/api

dev:
	@$(ENV) go tool air

test:
	@$(ENV) go test ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

db-up:
	$(COMPOSE) up -d --wait --remove-orphans

db-down:
	$(COMPOSE) down

migrate:
	@$(ENV) goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@$(ENV) goose -dir migrations postgres "$$DATABASE_URL" down

migration:
	@test -n "$(name)" || (echo "usage: make migration name=create_businesses" && exit 1)
	goose -dir migrations -s create $(name) sql

generate:
	@test -d queries || (echo "no queries/ yet: write the first query, then run make generate" && exit 1)
	sqlc generate
