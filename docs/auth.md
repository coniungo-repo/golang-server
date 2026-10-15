# Auth

Passwordless sign-in with SuperTokens: users enter an email or mobile number, receive a 6-digit code, and exchange it for a session. There are no passwords.

## How it fits together

```
app ──► golang-server ──► SuperTokens core ──► supertokens database
            │                                   (same Postgres server)
            ├──► email.Sender / sms.Sender  (delivers the code)
            └──► sign_in_attempts           (rate limits, app database)
```

- The **core** is self-hosted (`deploy/docker-compose.yml` locally). It owns users, codes and sessions in its own `supertokens` database, with its own login.
- The **API** serves the SuperTokens routes under `SUPERTOKENS_API_BASE_PATH` (`/auth`), validates input, sends codes through our own delivery packages, and rate limits sends.
- Your tables reference users by SuperTokens user ID. Never join across the two databases.

## Endpoints

Use the SuperTokens frontend SDKs (web, React Native, iOS, Android). They call:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/auth/signinup/code` | Send a code to `email` or `phoneNumber` |
| `POST` | `/auth/signinup/code/resend` | Resend the code |
| `POST` | `/auth/signinup/code/consume` | Exchange the code for a session |
| `POST` | `/auth/session/refresh` | Refresh the session |
| `POST` | `/auth/signout` | Sign out |
| `GET` | `/api/v1/me` | Example protected route; returns `user_id` |

Browsers get cookies. Native apps send `st-auth-mode: header` and receive tokens in headers.

The email and phone "exists" endpoints are disabled, so accounts can't be enumerated.

## Protecting routes

```go
mux.Handle("GET /orders", deps.Auth.RequireSession(http.HandlerFunc(listOrders)))

func listOrders(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
}
```

## Input rules

- Emails are trimmed, lowercased and must be a bare, routable address.
- Phones go through `platform/phone`: local input uses `PHONE_DEFAULT_REGION`, everything is stored as E.164, and only mobile-capable numbers are accepted. `0803 123 4567` and `+234 803 123 4567` are the same account.

## Rate limits

Code sends (create and resend) are limited per destination and per client IP. Every window applies, and a send is refused if any is full:

| Key | Setting | Default |
| --- | --- | --- |
| Email or phone | `AUTH_RATE_LIMIT_DESTINATION` | `5/15m,15/24h` |
| Client IP | `AUTH_RATE_LIMIT_CLIENT_IP` | `30/1h,100/24h` |

Counts live in `sign_in_attempts`, keyed by an HMAC with `FINGERPRINT_SECRET`, so raw emails, phones and IPs are never stored. Rows older than the longest window are pruned hourly. Set `HTTP_CLIENT_IP_HEADER` behind a proxy, or every user shares the proxy's IP.

The core separately locks a code after 5 wrong attempts, and codes expire after 10 minutes (`PASSWORDLESS_CODE_LIFETIME`, `PASSWORDLESS_MAX_CODE_INPUT_ATTEMPTS`).

## Delivery

`platform/email` and `platform/sms` define a `Sender` interface. The only provider today is `log`, which prints codes to the console for local work. Production refuses to start with it.

To add a provider:

1. Implement `Send(ctx, Message) error` in a new file, e.g. `platform/sms/termii.go`. Respect `ctx`, which carries a 10-second timeout.
2. Add its name to `emailProviders` or `smsProviders` in `platform/config/load.go`, along with any settings it needs (keys as `Secret`).
3. Return it from `newEmailSender` or `newSMSSender` in `internal/bootstrap/dependencies.go`.

Message copy lives in `internal/auth/delivery.go`. The SMS fits a single 160-character segment.

## Self-hosting the core

Run the image `registry.supertokens.io/supertokens/supertokens-postgresql`, pinned to the version in `deploy/docker-compose.yml`:

- `POSTGRESQL_CONNECTION_URI`: the `supertokens` database, owned by its own role. `deploy/postgres/init-supertokens.sh` creates both and is safe to rerun.
- `API_KEYS`: the same value as `SUPERTOKENS_API_KEY`.
- Keep the core on a private network. Only the API talks to it, never browsers or apps.
- Back up the `supertokens` database with the app database. Losing it signs everyone out and orphans user IDs.

Check SDK and core compatibility before upgrading either: the core's `/apiversion` must list the SDK's CDI version (3.1 for supertokens-golang v0.26).
