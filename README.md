# golang-server

HTTP API for Coniungo, written in Go with the standard library router and Postgres via pgx.

## Requirements

- Go 1.26.9 or later
- Docker, for local Postgres and image builds

## Quick start

```bash
cp .env.example .env
make db-up
make migrate
make dev
```

The server listens on `http://localhost:8080`. Check it with `curl localhost:8080/readyz`.

## Commands

| Command | What it does |
| --- | --- |
| `make dev` | Run with live reload (air) |
| `make run` | Run once |
| `make test` | Tests with the race detector |
| `make lint` | gofmt, go vet, staticcheck |
| `make vuln` | govulncheck |
| `make check` | lint, test and vuln, as CI runs them |
| `make build` | Static binary in `bin/api` |
| `make docker` | Production image tagged with the git version |
| `make db-up` / `make db-down` | Start or stop local Postgres |
| `make migration name=x` | New SQL migration in `migrations/` |
| `make migrate` / `make migrate-down` | Apply or roll back migrations |
| `make generate` | Generate typed queries from `queries/` with sqlc |

Dev tools (air, goose, sqlc, staticcheck, govulncheck) are pinned in `tools.mod` and run through `go tool`, so nothing needs installing globally.

## Layout

```
cmd/api                 entry point
internal/bootstrap      wiring: config, dependencies, router
internal/health         liveness and readiness probes
internal/platform       shared infrastructure (config, logging, middleware, HTTP helpers)
migrations              goose SQL migrations
queries                 sqlc query files
deploy                  local Docker Compose
```

## Documentation

- [Architecture](docs/architecture.md)
- [Configuration](docs/configuration.md)
- [Operations](docs/operations.md)
