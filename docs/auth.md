# Auth

SuperTokens proves who someone is and runs their sessions. Everything else (users, roles, permissions, admin invites) lives in our database.

| Who | Signs in with | Gets |
| --- | --- | --- |
| Customers and providers | A 6-digit code by SMS (or email) | The `customer` role on first sign-in; `provider` is granted after onboarding |
| Staff admins | Google Workspace (OpenID Connect), `@coniungolabs.com`, invite only | The `admin` role, on a separate admin host |

There are no passwords.

## How it fits together

```
app ──► golang-server ──► SuperTokens core ──► supertokens database
            │                                   (same Postgres server)
            ├──► email.Sender / sms.Sender  (delivers the code)
            └──► sign_in_attempts           (rate limits, app database)
```

- The **core** is self-hosted (`deploy/docker-compose.yml` locally). It owns users, codes and sessions in its own `supertokens` database, with its own login.
- The **API** serves the SuperTokens routes under `SUPERTOKENS_API_BASE_PATH` (`/auth`), validates input, sends codes through our own delivery packages, and rate limits sends.
- Your tables reference `users.id`, never a SuperTokens user ID, and never join across the two databases.

## Users, identities and roles

| Table | Holds |
| --- | --- |
| `users` | Our user ID (UUID) and `status` (`active` or `suspended`) |
| `user_identities` | Each verified way in: `phone` (E.164), `email` (lowercased) or `oidc` (`issuer|sub`), with its SuperTokens user ID |
| `user_roles` | `customer`, `provider`, `admin` |
| `admin_invites` | Staff emails allowed to sign in as admin, and who accepted each |
| `admin_audit_log` | Every invite, revoke, suspend and reactivate, with who did it |

On every sign-in the API resolves the SuperTokens user to our user and writes our user ID, roles, sign-in method and sign-in time into the session. Handlers read `auth.FromContext(ctx)` and never see SuperTokens IDs.

Identities are never linked automatically. Admin identities are keyed on the provider's `sub`, not the email, so a renamed staff email keeps its account and a reissued address gets nothing without a new invite.

## Endpoints

Use the SuperTokens frontend SDKs (web, React Native, iOS, Android). They call:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/auth/signinup/code` | Send a code to `email` or `phoneNumber` |
| `POST` | `/auth/signinup/code/resend` | Resend the code |
| `POST` | `/auth/signinup/code/consume` | Exchange the code for a session |
| `POST` | `/auth/session/refresh` | Refresh the session |
| `POST` | `/auth/signout` | Sign out |
| `GET` | `/auth/authorisationurl?thirdPartyId=staff` | Admin host only: start Google sign-in |
| `POST` | `/auth/signinup` | Admin host only: finish Google sign-in (code exchange only) |
| `GET` | `/api/v1/me` | The signed-in user's ID, roles and sign-in method |
| `GET`, `POST` | `/api/v1/admin/invites` | List or create admin invites |
| `POST` | `/api/v1/admin/invites/{id}/revoke` | Revoke an invite; removes admin access and signs them out |
| `POST` | `/api/v1/admin/users/{id}/suspend` | Suspend a user and sign them out |
| `POST` | `/api/v1/admin/users/{id}/reactivate` | Reactivate a user |

Browsers get cookies. Native apps send `st-auth-mode: header` and receive tokens in headers.

The email and phone "exists" endpoints are disabled, so accounts can't be enumerated.

## Protecting routes

```go
mux.Handle("GET /orders", deps.Auth.RequireSession(http.HandlerFunc(listOrders)))
mux.Handle("POST /listings", deps.Auth.RequireRole(users.RoleProvider, http.HandlerFunc(createListing)))
mux.Handle("POST /admin/x", deps.Auth.RequireAdmin(auth.PermManageUserStatus, http.HandlerFunc(adminThing)))

func listOrders(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
}
```

| Middleware | Allows |
| --- | --- |
| `RequireSession` | Any signed-in, active user |
| `RequireRole(role)` | Active users whose session holds `role` |
| `RequirePermission(perm)` | Active users whose roles grant `perm` (map in `internal/auth/middleware.go`) |
| `RequireAdmin(perm)` | Admin host only, Google session younger than `ADMIN_SESSION_MAX_AGE`, and `admin` re-checked in the database on every request |

Data rules ("only your own listing") belong in each handler's SQL, scoped to `auth.UserID(ctx)`.

## Revocation

Access tokens are checked by signature, so a revoked one still verifies until it expires. Three things close that gap:

- `RequireSession` checks the user is active on every request, so suspension applies immediately.
- `RequireAdmin` also re-checks the admin role, so revoking an admin applies immediately.
- The core issues 15-minute access tokens (`ACCESS_TOKEN_VALIDITY=900`). Role changes and sign-outs reach everything else within 15 minutes, because refresh fails once sessions are revoked.

Revoke sessions (`auth.Service.RevokeUserSessions`) whenever you change someone's roles.

## Admin sign-in

### Google setup (once)

1. In a Google Cloud project owned by the Workspace organisation, configure the OAuth consent screen as **Internal**. Google then refuses anyone outside the organisation.
2. Create an OAuth client of type **Web application** with the admin app's callback as the redirect URI, e.g. `https://admin.<brand>/auth/callback/staff`.
3. In the Workspace admin console, enforce 2-step verification, preferably passkeys or security keys, for staff with admin access.
4. Set `OIDC_ISSUER=https://accounts.google.com`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_HOSTED_DOMAIN` and `ADMIN_EMAIL_DOMAIN` to `coniungolabs.com`, and `ADMIN_API_HOST`.

### What is checked

The SDK verifies the ID token's signature and expiry. We also require the right issuer and audience, a verified email on `ADMIN_EMAIL_DOMAIN`, the Workspace `hd` claim, and either an open invite (first sign-in) or an existing active admin with that `sub`. Tokens posted directly by a client are refused; only the server-side code exchange is accepted.

### Admin host

Admin routes and Google sign-in only answer on `ADMIN_API_HOST` (required in production), so admin cookies never mix with customer cookies. Point the admin app at that host; it can be the same server.

### First admin and offboarding

```bash
make admin-invite email=you@coniungolabs.com
```

In production, run `/admin invite you@coniungolabs.com` from the image with the production environment. After that, admins invite each other in the admin app.

To offboard: suspend the person in Google, then revoke their invite in the admin app, which removes the role and signs them out everywhere.

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
