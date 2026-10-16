# Production checklist

Everything that must be set up, changed or verified before staging or production serves real users. Do staging first, with the same steps and its own accounts, secrets and database, then repeat for production.

Domains below are examples: `api.getgbam.com` (API), `app.getgbam.com` (customer app), `admin-api.getgbam.com` (admin API host) and `admin.getgbam.com` (admin app). Use `staging-` variants for staging.

The API refuses to start in production when a required setting is missing, still a placeholder or unsafe, and lists every problem at once. That catches most mistakes at deploy time; the steps below cover the rest.

## 1. Accounts and approvals

Start these first. Several take days.

| Service | What to do | You get | Lead time |
| --- | --- | --- | --- |
| Termii | Open an account. Apply for the sender ID (e.g. `GetGbam`). Ask support to enable the **DND route** | `TERMII_API_KEY`, `TERMII_BASE_URL` (dashboard) | Sender ID approval: days to weeks |
| Postmark | Create a server. Add the sending domain and publish its SPF, DKIM and DMARC records in Cloudflare DNS | `POSTMARK_SERVER_TOKEN` | Domain verification: minutes to a day; account approval can take longer |
| Cloudflare Turnstile | In the Cloudflare dashboard, add a Turnstile widget for each frontend hostname (managed or invisible mode) | the site key (for the apps) and `TURNSTILE_SECRET_KEY` | Minutes |
| Google Cloud | In a project owned by the Coniungo Workspace: OAuth consent screen **Internal**, OAuth client of type Web application, redirect URI `https://admin.getgbam.com/auth/callback/staff` | `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | Minutes |
| Google Workspace | Enforce 2-step verification (passkeys or security keys) for staff with admin access | | Minutes |
| Fly.io | Organisation, payment method, apps below | | Minutes |

## 2. Infrastructure

| Piece | Setup |
| --- | --- |
| API | `fly.staging.toml` and `fly.production.toml`: a Fly app from this repo's `Dockerfile` in London (`lhr`, the closest well-connected region to Nigeria), port `8080`, traffic gated on `GET /readyz`, rolling deploys, metrics under `[metrics]` on `9091`. Staging stops when idle; production keeps 2 machines running |
| Postgres | Two databases on one server: the app database and `supertokens`, each with its own role. `deploy/postgres/init-supertokens.sh` creates the second and is safe to rerun. TLS with a certificate the API can verify (`sslmode=verify-full`). Backups on, for both databases |
| SuperTokens core | `deploy/fly/supertokens.<env>.toml`: a separate Fly app running the image pinned in `deploy/docker-compose.yml`, reachable **only over Flycast** at `http://<core-app>.flycast`, with no public IP. Core settings in [section 5](#5-supertokens-core-settings) |
| DNS (Cloudflare) | `api.` and `admin-api.` point at the Fly API app as **DNS only (grey cloud), not proxied**. If proxied, `Fly-Client-IP` becomes a Cloudflare address and every user shares one rate limit. Certificates for both names come from Fly (`fly certs add`) |
| Frontend (Cloudflare) | Customer and admin apps on their own origins, calling the API with credentials |

## 3. Secrets to generate

Generate fresh values for each environment. Never reuse staging values in production, and never commit them.

| Setting | Generate with | Notes |
| --- | --- | --- |
| `FINGERPRINT_SECRET` | `openssl rand -hex 32` | Keys the rate-limit fingerprints. Changing it resets sign-in rate limits, nothing else |
| `SUPERTOKENS_API_KEY` | `openssl rand -hex 24` | Same value in the API and the core's `API_KEYS` |
| App and `supertokens` database passwords | `openssl rand -hex 24` | One role per database |

Set them as Fly secrets (`fly secrets set ...`), never in `fly.toml`.

## 4. API settings

Settings that are not secret already live in `fly.staging.toml` and `fly.production.toml`; secrets are set with `fly secrets set`. A test (`internal/platform/config/fly_test.go`) loads each file's `[env]` with stand-in secrets through the production validation, so the files can't drift from what the API accepts. Everything else keeps its default; see [configuration](configuration.md) for the full list.

### Must change from local

| Setting | Local | Production | Source |
| --- | --- | --- | --- |
| `APP_ENV` | `development` | `production` | Turns on every production check below, HSTS, JSON logs, and Swagger UI off |
| `DATABASE_URL` | local, `sslmode=disable` | managed Postgres URL with `sslmode=verify-full` | Postgres provider. Secret |
| `FINGERPRINT_SECRET` | placeholder | generated | Section 3. Secret |
| `SUPERTOKENS_CONNECTION_URI` | `http://localhost:3567` | `http://<core-app>.flycast` | Set in `fly.<env>.toml` |
| `SUPERTOKENS_API_KEY` | local value | generated | Section 3. Secret |
| `SUPERTOKENS_API_DOMAIN` | `http://localhost:8080` | `https://api.getgbam.com` | Your API domain |
| `SUPERTOKENS_WEBSITE_DOMAINS` | local apps | `https://app.getgbam.com,https://admin.getgbam.com` | Your frontend origins, https only |
| `CORS_ALLOWED_ORIGINS` | local apps | the same origins as `SUPERTOKENS_WEBSITE_DOMAINS`, and nothing else | |
| `HTTP_CLIENT_IP_HEADER` | empty | `Fly-Client-IP` | Without it every user shares the proxy's IP and its rate limit |
| `HTTP_DRAIN_DELAY` | `0s` | `5s` | Lets Fly stop routing before shutdown, so deploys drop no requests |
| `METRICS_ADDR` | empty | `:9091` | Matches `[metrics]` in `fly.toml` |
| `DB_MAX_CONNS` | `10` | at least `10`, within the Postgres plan's connection limit across all instances | |

### Delivery

| Setting | Production | Source |
| --- | --- | --- |
| `EMAIL_PROVIDER` | `postmark` | |
| `POSTMARK_SERVER_TOKEN` | the server's API token | Postmark. Secret |
| `EMAIL_FROM` | e.g. `GetGbam <hello@getgbam.com>`, on the verified domain | |
| `SMS_PROVIDER` | `termii` | |
| `TERMII_API_KEY` | API key | Termii. Secret |
| `TERMII_BASE_URL` | your account's base URL, https | Termii dashboard |
| `SMS_SENDER` | the approved sender ID, e.g. `GetGbam` | Termii |
| `TERMII_CHANNEL` | `dnd` (required in production) | DND route enabled by Termii support |
| `PHONE_DEFAULT_REGION` | `NG` | |
| `PHONE_ALLOWED_REGIONS` | `NG` (required in production) | Every allowed country is paid SMS; others sign in with email |

### Admin sign-in

| Setting | Production |
| --- | --- |
| `OIDC_ISSUER` | `https://accounts.google.com` |
| `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | from the Google OAuth client. Secret |
| `OIDC_HOSTED_DOMAIN`, `ADMIN_EMAIL_DOMAIN` | `coniungolabs.com` |
| `ADMIN_API_HOST` | `admin-api.getgbam.com` (required in production) |

### Check, but usually keep

| Setting | Default | When to change |
| --- | --- | --- |
| `SUPERTOKENS_APP_NAME` | | The brand in codes: `GetGbam` |
| `APP_NAME`, `LOG_LEVEL` | `golang-server`, `info` | Service name in logs; `debug` only while investigating |
| `RATE_LIMIT_CLIENT_IP`, `RATE_LIMIT_USER` | `600/1m`, `120/1m,3000/1h` | Raise if `rate_limited_total` shows real users, e.g. one carrier address |
| `AUTH_RATE_LIMIT_DESTINATION`, `AUTH_RATE_LIMIT_CLIENT_IP` | `5/15m,15/24h`, `30/1h,100/24h` | Code-send limits; tighten if SMS costs spike |
| `AUTH_RATE_LIMIT_SMS_TOTAL` | `1000/1h,5000/24h` | The ceiling on texts to new numbers. The default suits an unknown launch; set up the alerts in [operations](operations.md#sms-spend) and raise it when they fire |
| `HTTP_REQUEST_ID_HEADER` | empty | Leave empty unless the proxy is known to strip the header from clients |
| `API_DOCS_ENABLED` | `false` in production | Only enable where the API may be public anyway |

## 5. SuperTokens core settings

Set on the core's Fly app:

| Setting | Value | Why |
| --- | --- | --- |
| `POSTGRESQL_CONNECTION_URI` | the `supertokens` database with its own role | Secret |
| `API_KEYS` | the same value as the API's `SUPERTOKENS_API_KEY` | Secret |
| `ACCESS_TOKEN_VALIDITY` | `900` | 15-minute access tokens bound how long a revoked session keeps working |
| `PASSWORDLESS_CODE_LIFETIME` | `600000` | Codes expire after 10 minutes; the messages say so |
| `PASSWORDLESS_MAX_CODE_INPUT_ATTEMPTS` | `5` | Wrong guesses before a code is locked |

Check SDK and core compatibility before upgrading either: the core's `/apiversion` must list the SDK's CDI version.

## 6. First deploy, in order

Shown for staging; use the production files, app names and targets for production. First replace the example domains in `fly.<env>.toml` with the real ones.

1. Create both databases and roles; turn on backups.
2. Create the core, give it a private address only, set its secrets and deploy it:

   ```bash
   fly apps create getgbam-supertokens-staging
   fly ips allocate-v6 --private -a getgbam-supertokens-staging
   fly secrets set -a getgbam-supertokens-staging --stage POSTGRESQL_CONNECTION_URI='postgresql://…/supertokens' API_KEYS='…'
   make deploy-core-staging
   fly ips list -a getgbam-supertokens-staging
   ```

   The last command must show only the private address. Release any public one with `fly ips release`.
3. Migrations run on every API deploy (`release_command = "/migrate"` in `fly.<env>.toml`), before the new version takes traffic, so there is nothing to run by hand. A failed migration stops the deploy; `fly logs` shows why.
4. Create the API, set its secrets and deploy it:

   ```bash
   fly apps create getgbam-api-staging
   fly secrets set -a getgbam-api-staging --stage \
     DATABASE_URL='postgres://…?sslmode=verify-full' FINGERPRINT_SECRET='…' SUPERTOKENS_API_KEY='…' \
     OIDC_CLIENT_ID='…' OIDC_CLIENT_SECRET='…' POSTMARK_SERVER_TOKEN='…' TERMII_API_KEY='…' TERMII_BASE_URL='https://…' \
     TURNSTILE_SECRET_KEY='…'
   make deploy-staging
   fly certs add staging-api.getgbam.com -a getgbam-api-staging
   fly certs add staging-admin-api.getgbam.com -a getgbam-api-staging
   ```

   If the API refuses to start, `fly logs` lists each problem. Point the two DNS names at the app in Cloudflare (DNS only) as `fly certs` instructs. For production, `make deploy-production` refuses to run with uncommitted changes.
5. Invite the first admin: `fly ssh console -C "/admin invite you@coniungolabs.com"` runs the admin tool inside the API app with its production settings.

## 7. Verify before real users

| Check | Expect |
| --- | --- |
| `GET https://api.getgbam.com/livez` and `/readyz` | `200` |
| Phone sign-in on MTN, Airtel, Glo and 9mobile numbers, including one on DND | Code arrives within seconds from the sender ID; sign-in works |
| Code request without a Turnstile token | Refused with "Please complete the check"; `bot_checks_total{result="failed"}` goes up |
| Phone sign-in with a non-Nigerian number | Refused with the "use your email" message |
| Email sign-in | Code arrives in the inbox, not spam; check the headers pass SPF, DKIM and DMARC |
| Admin Google sign-in on `admin.getgbam.com` | Works for the invited admin; refused for an uninvited staff account and on the customer host |
| Request headers | `X-Request-ID` on every response; HSTS present; `/docs/` returns 404 |
| Rate limits | Hammering one route returns `429` with `Retry-After`; the `client_ip` limit counts your real IP (check `fly logs`) |
| Metrics | Fly's Grafana shows `http_requests_total`, `code_deliveries_total` and `db_pool_connections` |
| Deploy under traffic | A redeploy while requests run produces no 5xx |
| Logs | JSON lines with `request_id`; no codes, phone numbers or emails in them |
| Backups | A restore of both databases works |

## 8. After launch

- Set the starter alerts from [operations](operations.md#metrics): 5xx rate, delivery failures, pool saturation, core errors and code-send abuse.
- Watch Termii and Postmark balances and spend.
- Rotate a secret by setting the new value and redeploying. Rotating `SUPERTOKENS_API_KEY` needs both values in the core's `API_KEYS` (comma-separated) during the switch.
- Keep [operations: security checklist](operations.md#security-checklist) true as things change.
