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

`/readyz` shares one database ping per second across all callers, so frequent checks (or a flood, since health probes are exempt from rate limiting) never hold more than one pool connection.

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
- `PHONE_ALLOWED_REGIONS` lists only the countries you serve (required); phone sign-in already accepts mobile numbers only, never premium-rate or VoIP
- The core runs with `ACCESS_TOKEN_VALIDITY=900`
- Admin sign-in uses an Internal OAuth consent screen, enforced 2-step verification, and its own `ADMIN_API_HOST`
- TLS is terminated by the platform; the app sends HSTS in production
- CI is green: lint, race tests, govulncheck and image build

## Rate limiting

Every route except `/livez` and `/readyz` is limited per client IP (`RATE_LIMIT_CLIENT_IP`), with IPv6 grouped by /64. Routes that need a session are also limited per user (`RATE_LIMIT_USER`). Over either limit the API answers `429` with `Retry-After` and logs `rate limited` at warn level.

These counts are in memory, so each instance enforces the limits on its own: with 3 instances a client can make up to 3 times the limit, and a restart resets them. The per-IP limit is generous because mobile carriers put many users behind one address; set `HTTP_CLIENT_IP_HEADER` behind a proxy, or every user shares the proxy's IP. Volumetric attacks belong at the edge (proxy or CDN), not here.

Sign-in code sends have their own stricter limits, shared by every instance through Postgres. See [auth](auth.md#rate-limits).

## Not yet in place

- Metrics and tracing
