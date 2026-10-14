# Configuration

All configuration comes from environment variables. The server validates everything at startup and exits listing every problem at once. `.env.example` has working local values.

## App

| Variable | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `development`, `test` or `production` |
| `APP_NAME` | `golang-server` | Added to every log line |
| `APP_VERSION` | `unknown` | Set by the Docker build |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## HTTP

| Variable | Default | Notes |
| --- | --- | --- |
| `HTTP_HOST` | `0.0.0.0` | |
| `PORT` | `8080` | |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | |
| `HTTP_READ_TIMEOUT` | `15s` | |
| `HTTP_WRITE_TIMEOUT` | `30s` | |
| `HTTP_IDLE_TIMEOUT` | `60s` | |
| `HTTP_REQUEST_TIMEOUT` | `20s` | Must be shorter than the write timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | Total time allowed for shutdown |
| `HTTP_DRAIN_DELAY` | `0s` | Time `/readyz` reports 503 before connections close. Set to about 5s behind a load balancer |
| `HTTP_REQUEST_ID_HEADER` | empty | Reuse a proxy's request ID, e.g. `Fly-Request-Id`. Only set this when that proxy strips the header from clients |

## Database

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | required | Production requires `sslmode=verify-full` |
| `DB_MAX_CONNS` | `25` | 1 to 1000; at least 10 in production |
| `DB_MIN_CONNS` | `2` | Connections kept open; at most `DB_MAX_CONNS` |
| `DB_CONN_MAX_IDLE` | `15m` | |

## Security

| Variable | Default | Notes |
| --- | --- | --- |
| `CORS_ALLOWED_ORIGINS` | none | Comma-separated `scheme://host[:port]`. No wildcards, paths or trailing slashes |
| `FINGERPRINT_SECRET` | required | At least 32 characters. Generate with `openssl rand -hex 32`. The example placeholder is rejected in production |
