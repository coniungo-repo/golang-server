# Configuration

All configuration comes from environment variables. The server validates everything at startup and exits listing every problem at once. `.env.example` has working local values.

## App

| Variable | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `development`, `test` or `production` |
| `APP_NAME` | `golang-server` | Added to every log line |
| `APP_VERSION` | `unknown` | Set by the Docker build |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `API_DOCS_ENABLED` | `true` outside production, `false` in production | Serves `/openapi.yaml` and Swagger UI at `/docs/` |

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
| `METRICS_ADDR` | empty (off) | Where to serve Prometheus metrics, e.g. `:9091`. A separate port from `PORT`; never expose it publicly |
| `HTTP_MAX_HEADER_BYTES` | `65536` | Largest request header block, 4096 to 1048576 |
| `HTTP_REQUEST_ID_HEADER` | empty | Reuse a proxy's request ID, e.g. `Fly-Request-Id`. Only set this when that proxy strips the header from clients |
| `RATE_LIMIT_CLIENT_IP` | `600/1m` | Requests per client IP (IPv6 per /64) on every route but `/livez` and `/readyz`, as `limit/period` pairs. Up to 4 windows, each 1s to 168h. Per instance |
| `RATE_LIMIT_USER` | `120/1m,3000/1h` | Requests per signed-in user on session routes, same format. Per instance |

## Database

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | required | Production requires `sslmode=verify-full` |
| `DB_MAX_CONNS` | `25` | 1 to 1000; at least 10 in production |
| `DB_MIN_CONNS` | `2` | Connections kept open; at most `DB_MAX_CONNS` |
| `DB_CONN_MAX_IDLE` | `15m` | |
| `DB_CONNECT_TIMEOUT` | `5s` | Opening a connection, and the startup ping |
| `DB_HEALTH_CHECK_PERIOD` | `1m` | How often the pool checks idle connections |
| `DB_READY_TIMEOUT` | `2s` | The `/readyz` ping. Shorter than `HTTP_REQUEST_TIMEOUT` |

## Security

| Variable | Default | Notes |
| --- | --- | --- |
| `CORS_ALLOWED_ORIGINS` | local apps outside production, none in production | Comma-separated `scheme://host[:port]`. No wildcards, paths or trailing slashes |
| `CORS_MAX_AGE` | `10m` | How long browsers may cache a preflight, at most `24h` |
| `FINGERPRINT_SECRET` | required | At least 32 characters. Generate with `openssl rand -hex 32`. The example placeholder is rejected in production |

## Auth (SuperTokens)

| Variable | Default | Notes |
| --- | --- | --- |
| `SUPERTOKENS_CONNECTION_URI` | `http://localhost:3567` outside production | SuperTokens core. Separate several with `;`. Plain http is allowed for a private network |
| `SUPERTOKENS_API_KEY` | empty | At least 20 characters, must match the core's `API_KEYS`. Required in production |
| `SUPERTOKENS_APP_NAME` | required | The brand shown in emails and SMS |
| `SUPERTOKENS_API_DOMAIN` | `http://localhost:$PORT` outside production | Public origin of this API. https in production |
| `SUPERTOKENS_API_BASE_PATH` | `/auth` | Where the auth routes are mounted |
| `SUPERTOKENS_WEBSITE_DOMAINS` | local apps outside production | Comma-separated frontend origins. Each must also be in `CORS_ALLOWED_ORIGINS`. https in production |
| `SUPERTOKENS_WEBSITE_BASE_PATH` | `/auth` | Frontend auth route |
| `SUPERTOKENS_TIMEOUT` | `5s` | Limit on each call to the core. The SDK ignores request deadlines, so this is what stops a hung core from holding requests and database locks. Must be shorter than `HTTP_REQUEST_TIMEOUT` |
| `AUTH_RATE_LIMIT_DESTINATION` | `5/15m,15/24h` | Code sends per email or phone, as `limit/period` pairs. Up to 4 windows, each 1s to 168h |
| `AUTH_RATE_LIMIT_CLIENT_IP` | `30/1h,100/24h` | Code sends per client IP, same format |
| `AUTH_RATE_LIMIT_SMS_TOTAL` | `300/1h,2000/24h` | Texts across every number and client, shared by all instances: the ceiling on SMS spend. Over it, phone sign-in is refused with a message pointing to email. Raise it as real traffic grows |

Local Docker Compose also reads `SUPERTOKENS_DB_USER`, `SUPERTOKENS_DB_PASSWORD` and `SUPERTOKENS_DB_NAME` to create the core's database. The API never uses them.

## Admin sign-in

Leave `OIDC_ISSUER` empty to disable admin sign-in.

| Variable | Default | Notes |
| --- | --- | --- |
| `OIDC_ISSUER` | empty | `https://accounts.google.com` for Google Workspace. https only, except loopback outside production |
| `OIDC_CLIENT_ID` | empty | OAuth client ID |
| `OIDC_CLIENT_SECRET` | empty | OAuth client secret |
| `OIDC_HOSTED_DOMAIN` | empty | Google Workspace domain; required in the `hd` claim. Must equal `ADMIN_EMAIL_DOMAIN` |
| `ADMIN_EMAIL_DOMAIN` | empty | Staff email domain, e.g. `coniungolabs.com`. Invites must use it |
| `ADMIN_API_HOST` | empty | Host that serves admin routes and Google sign-in. Required in production |
| `ADMIN_SESSION_MAX_AGE` | `12h` | How long after Google sign-in admin routes keep working. At most 168h |

## Delivery

| Variable | Default | Notes |
| --- | --- | --- |
| `EMAIL_PROVIDER` | `log` | `postmark`, or `log`, which prints codes to the console and is rejected in production |
| `EMAIL_FROM` | empty | e.g. `"GetGbam <hello@getgbam.com>"` (quote it in `.env`). On a domain verified in Postmark. Required in production and with `postmark` |
| `POSTMARK_SERVER_TOKEN` | empty | Server API token. Required with `postmark` |
| `POSTMARK_MESSAGE_STREAM` | `outbound` | Postmark's default transactional stream |
| `SMS_PROVIDER` | `log` | `termii`, or `log` as above |
| `SMS_SENDER` | empty | Termii sender ID: 3 to 11 letters, digits or spaces, approved by Termii, e.g. `GetGbam` |
| `TERMII_API_KEY` | empty | Required with `termii` |
| `TERMII_BASE_URL` | empty | Your account's API base URL from the Termii dashboard, https. Required with `termii` |
| `TERMII_CHANNEL` | `dnd` | `dnd` reaches numbers on Do-Not-Disturb and must be enabled by Termii support; `generic` is for promotions and is refused in production |
| `DELIVERY_TIMEOUT` | `10s` | Limit on each email or SMS send. Shorter than `HTTP_REQUEST_TIMEOUT` |

## Phone numbers

| Variable | Default | Notes |
| --- | --- | --- |
| `PHONE_DEFAULT_REGION` | empty | ISO 3166-1 alpha-2 code, e.g. `NG`. Lets users omit the country code. Empty requires `+` |
| `PHONE_ALLOWED_REGIONS` | all outside production, required in production | Comma-separated region codes to accept. Must include the default region. Only the countries you send codes to, since every code is a paid SMS |

## Fixed in code

These are deliberately not settings:

| Value | Where | Why |
| --- | --- | --- |
| 1 MiB request body limit | `platform/request.MaxBodyBytes` | Part of the API contract in `api/openapi.yaml`; clients rely on it |
| HSTS two years with subdomains, the other security headers | `platform/middleware/headers.go` | Security policy, not tuning |
| Admin provider ID `staff`, OIDC scopes and `prompt=select_account` | `auth/admin_signin.go` | Frontend contract and sign-in security |
| Rate limiter sweep (5m) and sign-in attempt pruning (1h) | `bootstrap/api.go` | Housekeeping; memory and table size do not depend on them meaningfully |
| `/readyz` ping reuse (1s) | `health` | Protects the pool; no reason to vary |
| Validation bounds (secret lengths, window counts, pool size) | `platform/config` | Guard rails on the settings themselves |
| Messages and email/SMS copy | `auth`, `platform/phone` | Product copy, changed with code review |

Code lifetime, code attempts and access token lifetime are settings of the SuperTokens core (`PASSWORDLESS_CODE_LIFETIME`, `PASSWORDLESS_MAX_CODE_INPUT_ATTEMPTS`, `ACCESS_TOKEN_VALIDITY`), not this API.
