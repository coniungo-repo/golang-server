# Architecture

## Request flow

Every request passes through one middleware chain, in this order:

1. **RequestID**: assigns an ID, adds it to the logger and the `X-Request-ID` response header
2. **Log**: one structured line per request with method, matched route, status, bytes and duration
3. **Routes**: records the matched pattern so logs stay low-cardinality
4. **Recover**: turns panics into a JSON 500, or aborts the connection if the response already started
5. **SecureHeaders**: nosniff, frame denial, strict CSP, no-store, and HSTS in production
6. **CORS**: credentialed CORS for listed origins, plus CSRF protection via `http.CrossOriginProtection`
7. **MaxBytes**: 1 MiB body limit
8. **Timeout**: request context deadline

The SuperTokens middleware sits inside the chain and serves `/auth/*`. Everything else falls through to the router.

Unknown paths return a JSON 404. Known paths with the wrong method return a JSON 405 with `Allow`.

## Adding an endpoint

Create a package under `internal/`, expose a `Register(mux route.Mux)` function, and call it from `registerAPI` in `internal/bootstrap/router.go`. Routes registered there are served under `/api/v1`.

```go
func (h *Handler) Register(mux route.Mux) {
	mux.Handle("POST /things", http.HandlerFunc(h.create))
}
```

Handlers should:

- decode bodies with `request.Decode`, and answer a `*request.Error` with its `Status` and `Message`
- pass `r.Context()` to every database and network call
- send results with `respond.JSON` and failures with `respond.Error` or `respond.ServerError`

Phone numbers go through the shared validator in `deps.Phone`, never straight into storage:

```go
n, err := h.phones.Parse(body.Phone)
if err != nil {
	respond.Error(w, r, http.StatusUnprocessableEntity, phone.Message(err))
	return
}
store(n.E164())
```

`Parse` accepts local or international input, including full-width digits and common separators, and rejects letters, extensions, unknown country codes, impossible lengths and unassigned ranges. By default only mobile-capable numbers pass. `Number` masks itself in `fmt` and slog; `E164()` is the only way to the raw value.

`respond.ServerError` logs the cause and sends a generic message. Deadlines become 503 and client disconnects 499.

## Packages

| Package | Responsibility |
| --- | --- |
| `platform/config` | Load and validate environment variables; `Secret` keeps values out of logs |
| `platform/logger` | slog setup (JSON in production) and credential redaction |
| `platform/database` | pgx pool with startup ping |
| `platform/httpserver` | Server lifecycle and graceful shutdown |
| `platform/middleware` | The chain above |
| `auth` | SuperTokens passwordless and sessions, code delivery, rate limiting. See [auth](auth.md) |
| `users` | Users, identities, roles, admin invites and the audit log |
| `store` | sqlc-generated queries |
| `platform/email` | Email `Sender` interface, address normalisation, log provider |
| `platform/sms` | SMS `Sender` interface, log provider |
| `platform/phone` | Phone number parsing, policy and masking |
| `platform/request` | Strict JSON decoding |
| `platform/respond` | JSON responses and error mapping |
| `platform/requestid` | Request ID generation and context storage |
| `platform/route` | Path prefixing for route groups |
| `platform/clock` | Injectable time source for tests |
