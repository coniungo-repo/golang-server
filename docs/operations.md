# Operations

## Build and deploy

```bash
make docker VERSION=v1.2.3
```

The image is a static binary on distroless, running as a non-root user with `APP_ENV=production`. Provide the variables in [configuration](configuration.md) through your platform's secret store, never in the image.

Run `make migrate` against the target database before rolling out code that depends on a new migration. Migrations should be backward compatible with the version currently running.

## SuperTokens core

Deploy the core alongside the API on a private network, pointed at its own `supertokens` database on the same Postgres server. See [auth](auth.md#self-hosting-the-core).

## Health checks

| Endpoint | Use | Fails when |
| --- | --- | --- |
| `GET /livez` | Liveness | The process can't serve HTTP |
| `GET /readyz` | Readiness | Postgres is unreachable or the server is draining |

Point restarts at `/livez` only. A database outage should take instances out of rotation, not restart them.

## Shutdown

On `SIGTERM` or `SIGINT` the server:

1. fails `/readyz`
2. waits `HTTP_DRAIN_DELAY` so the load balancer stops sending traffic
3. stops accepting connections and waits for in-flight requests until `HTTP_SHUTDOWN_TIMEOUT`
4. force-closes anything left, then closes the database pool

A second signal exits immediately.

## Logging

Production logs are JSON on stdout, one line per request, with `request_id`, `route`, `status` and `duration`. Clients see the same `request_id` in the `X-Request-ID` header and in error bodies, so support reports can be matched to logs.

Keys containing `password`, `secret`, `token`, `authorization`, `cookie`, `api_key`, `private_key`, `dsn` or `database_url` are redacted. Wrap credentials in `config.Secret` regardless.

## Security checklist

- `DATABASE_URL` uses `sslmode=verify-full`
- `CORS_ALLOWED_ORIGINS` lists only first-party origins
- `FINGERPRINT_SECRET` is unique per environment and stored as a secret
- `HTTP_REQUEST_ID_HEADER` and `HTTP_CLIENT_IP_HEADER` are set only behind a proxy that overwrites them
- `SUPERTOKENS_API_KEY` is unique per environment and the core is not publicly reachable
- Real email and SMS providers are configured (`log` is rejected)
- TLS is terminated by the platform; the app sends HSTS in production
- CI is green: lint, race tests, govulncheck and image build

## Not yet in place

- Authorisation (roles and permissions)
- General API rate limiting; only sign-in code sends are limited today
- Metrics and tracing
