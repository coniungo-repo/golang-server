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
| `HTTP_CLIENT_IP_HEADER` | empty | Proxy header carrying the client IP, e.g. `Fly-Client-IP`, used for rate limiting. Only set behind a proxy that overwrites it |
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
| `CORS_ALLOWED_ORIGINS` | local apps outside production, none in production | Comma-separated `scheme://host[:port]`. No wildcards, paths or trailing slashes |
| `FINGERPRINT_SECRET` | required | At least 32 characters. Generate with `openssl rand -hex 32`. The example placeholder is rejected in production |

## Auth (SuperTokens)

| Variable | Default | Notes |
| --- | --- | --- |
| `SUPERTOKENS_CONNECTION_URI` | `http://localhost:3567` outside production | SuperTokens core. Separate several with `;`. Plain http is allowed for a private network |
| `SUPERTOKENS_API_KEY` | empty | At least 20 characters, must match the core's `API_KEYS`. Required in production |
| `SUPERTOKENS_APP_NAME` | `GetGbam` | Shown in emails and SMS |
| `SUPERTOKENS_API_DOMAIN` | `http://localhost:$PORT` outside production | Public origin of this API. https in production |
| `SUPERTOKENS_API_BASE_PATH` | `/auth` | Where the auth routes are mounted |
| `SUPERTOKENS_WEBSITE_DOMAINS` | local apps outside production | Comma-separated frontend origins. Each must also be in `CORS_ALLOWED_ORIGINS`. https in production |
| `SUPERTOKENS_WEBSITE_BASE_PATH` | `/auth` | Frontend auth route |

| `AUTH_RATE_LIMIT_DESTINATION` | `5/15m,15/24h` | Code sends per email or phone, as `limit/period` pairs. Up to 4 windows, each 1s to 168h |
| `AUTH_RATE_LIMIT_CLIENT_IP` | `30/1h,100/24h` | Code sends per client IP, same format |

Local Docker Compose also reads `SUPERTOKENS_DB_USER`, `SUPERTOKENS_DB_PASSWORD` and `SUPERTOKENS_DB_NAME` to create the core's database. The API never uses them.

## Delivery

| Variable | Default | Notes |
| --- | --- | --- |
| `EMAIL_PROVIDER` | `log` | `log` prints codes to the console and is rejected in production |
| `EMAIL_FROM` | empty | e.g. `"GetGbam <hello@getgbam.com>"` (quote it in `.env`). Required in production |
| `SMS_PROVIDER` | `log` | As above |
| `SMS_SENDER` | empty | Sender ID or number, as your provider requires |

## Phone numbers

| Variable | Default | Notes |
| --- | --- | --- |
| `PHONE_DEFAULT_REGION` | empty | ISO 3166-1 alpha-2 code, e.g. `NG`. Lets users omit the country code. Empty requires `+` |
| `PHONE_ALLOWED_REGIONS` | all | Comma-separated region codes to accept. Must include the default region |
