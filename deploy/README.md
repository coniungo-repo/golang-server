# Deploy files

| File | Used by | What it is |
| --- | --- | --- |
| `docker-compose.yml` | `make db-up`, local development | Postgres and the SuperTokens core on your machine. The core image version here is the one every environment runs |
| `postgres/init-supertokens.sh` | compose, and once per new database server | Creates the core's own `supertokens` database and role. Safe to rerun |
| `fly/supertokens.staging.toml`, `fly/supertokens.production.toml` | `make deploy-core-staging`, `make deploy-core-production` | The SuperTokens core on Fly, reachable only over Flycast, pinned by digest |
| `../fly.staging.toml`, `../fly.production.toml` | `make deploy-staging`, `make deploy-production` | The API on Fly. At the repository root because Fly builds from the root |
| `../Dockerfile` | every API deploy, CI | The API image: `/api` (default), `/migrate` (Fly's release command) and `/admin` |

Changing the core version: edit `docker-compose.yml` and both `fly/supertokens.*.toml` (tag and digest) together. `TestFlyCoreMatchesCompose` fails if they differ.

Changing an API setting: settings that aren't secret go in `[env]` of `fly.<env>.toml`; secrets go in `fly secrets set`. `TestFlyConfigsPassProductionValidation` runs each file through the production checks.

The full first-deploy checklist is [docs/production.md](../docs/production.md).
