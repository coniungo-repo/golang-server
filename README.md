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

`make db-up` starts Postgres and a self-hosted SuperTokens core. The server listens on `http://localhost:8080`; check it with `curl localhost:8080/readyz`. Sign-in codes are printed to the console until real email and SMS providers are configured.

Open [localhost:8080/docs/](http://localhost:8080/docs/) to explore and call the API in Swagger UI, including signing in with a code from the console.

## Commands

| Command | What it does |
| --- | --- |
| `make dev` | Run with live reload (air) |
| `make run` | Run once |
| `make test` | Tests with the race detector |
| `make test-db` | All tests, including Postgres and SuperTokens end-to-end ones, against your local stack |
| `make lint` | gofmt, go vet, staticcheck |
| `make vuln` | govulncheck |
| `make check` | lint, test and vuln, as CI runs them |
| `make build` | Static binary in `bin/api` |
| `make docker` | Production image tagged with the git version |
| `make db-up` / `make db-down` | Start or stop local Postgres and the SuperTokens core |
| `make migration name=x` | New SQL migration in `migrations/` |
| `make migrate` / `make migrate-down` | Apply or roll back migrations |
| `make admin-invite email=x` | Invite a staff admin (needs admin sign-in configured) |
| `make generate` | Generate typed queries from `queries/` with sqlc |

Dev tools (air, goose, sqlc, staticcheck, govulncheck) are pinned in `tools.mod` and run through `go tool`, so nothing needs installing globally.

## Layout

```
api                     OpenAPI description of every route
cmd/api                 API server
cmd/admin               admin invite CLI
internal/bootstrap      wiring: config, dependencies, router
internal/auth           sign-in, sessions, admin sign-in, authorisation, rate limiting
internal/users          users, identities, roles, admin invites
internal/health         liveness and readiness probes
internal/docs           Swagger UI (vendored swagger-ui-dist) for api/openapi.yaml
internal/store          sqlc-generated queries
internal/platform       shared infrastructure (config, logging, middleware, HTTP helpers)
migrations              goose SQL migrations
queries                 sqlc query files
deploy                  local Docker Compose
```

## Documentation

- [API reference](api/openapi.yaml), browsable at `/docs/` locally
- [Architecture](docs/architecture.md)
- [Auth](docs/auth.md)
- [Configuration](docs/configuration.md)
- [Operations](docs/operations.md)
