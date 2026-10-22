#!/usr/bin/env bash
# Exercises every API route against the local Postgres and SuperTokens core (make db-up), like a client would.
# It builds and starts its own server on SMOKE_PORT with codes printed to its log, reads the codes from there,
# and stops the server when done. Run with `make smoke`. Exit status is non-zero if any check fails.
set -uo pipefail

API_PORT="${SMOKE_PORT:-18080}"
METRICS_PORT="${SMOKE_METRICS_PORT:-19091}"
BASE="http://localhost:${API_PORT}"
WORK="$(mktemp -d)"
LOG="${WORK}/api.log"
PASS=0
FAIL=0

cleanup() {
	[ -n "${SERVER_PID:-}" ] && kill "${SERVER_PID}" 2>/dev/null && wait "${SERVER_PID}" 2>/dev/null
	if [ "${FAIL}" -gt 0 ]; then echo "server log kept at ${LOG}"; else rm -rf "${WORK}"; fi
}
trap cleanup EXIT

for tool in curl jq go; do
	command -v "$tool" >/dev/null || { echo "missing $tool"; exit 1; }
done
[ -f .env ] || { echo "run from the repository root, with a .env (cp .env.example .env)"; exit 1; }

set -a
. ./.env
set +a
export PORT="${API_PORT}" METRICS_ADDR=":${METRICS_PORT}" SUPERTOKENS_API_DOMAIN="${BASE}" APP_ENV=development \
	EMAIL_PROVIDER=log SMS_PROVIDER=log BOT_CHECK="${SMOKE_BOT_CHECK:-off}" PHONE_DEFAULT_REGION=NG PHONE_ALLOWED_REGIONS=NG \
	AUTH_RATE_LIMIT_CLIENT_IP=100000/1h
[ "${BOT_CHECK}" = "turnstile" ] && export TURNSTILE_SECRET_KEY=1x0000000000000000000000000000000AA
TOKEN_HEADER="X-Turnstile-Token: XXXX.DUMMY.TOKEN.XXXX"
NO_TOKEN=""

echo "building and starting the API on ${BASE} (bot check: ${BOT_CHECK})"
go build -o "${WORK}/api" ./cmd/api || exit 1
"${WORK}/api" >"${LOG}" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do curl -sf "${BASE}/livez" >/dev/null && break; sleep 0.5; done
curl -sf "${BASE}/livez" >/dev/null || { echo "server did not start:"; tail -20 "${LOG}"; exit 1; }

# req METHOD PATH [TOKEN] [JSON BODY] [HOST]: sets STATUS, BODY and HDRS.
req() {
	local method="$1" path="$2" token="${3:-}" body="${4:-}" host="${5:-}"
	local args=(-s -o "${WORK}/body" -D "${WORK}/headers" -w '%{http_code}' -X "$method" "${BASE}${path}"
		-H 'Content-Type: application/json' -H 'st-auth-mode: header')
	[ -z "$NO_TOKEN" ] && args+=(-H "${TOKEN_HEADER}")
	[ -n "$token" ] && args+=(-H "Authorization: Bearer ${token}")
	[ -n "$body" ] && args+=(--data-raw "$body")
	[ -n "$host" ] && args+=(-H "Host: ${host}")
	if [[ "$path" == /auth/signinup && "$method" == POST ]]; then args+=(-H 'rid: thirdparty'); fi
	STATUS="$(curl "${args[@]}")"
	BODY="$(cat "${WORK}/body")"
	HDRS="$(tr -d '\r' <"${WORK}/headers")"
}

check() {
	local name="$1"
	shift
	if "$@"; then
		PASS=$((PASS + 1))
		printf '  \033[32mok\033[0m    %s\n' "$name"
	else
		FAIL=$((FAIL + 1))
		printf '  \033[31mFAIL\033[0m  %s\n        status %s body %s\n' "$name" "$STATUS" "$BODY"
	fi
}

is() { [ "$STATUS" = "$1" ]; }
field() { jq -r "$1" <<<"$BODY" 2>/dev/null; }
has_field() { [ "$(field "$1")" = "$2" ]; }
header() { grep -i "^$1:" <<<"$HDRS" | head -1 | cut -d' ' -f2-; }
code_for() { grep "to=$1 " "${LOG}" | tail -1 | grep -oE '(body|subject)="[0-9]{6} ' | grep -oE '[0-9]{6}'; }

PHONE_LOCAL="080$(printf '%08d' $((RANDOM * RANDOM % 100000000)))"
PHONE="+234${PHONE_LOCAL:1}"
EMAIL="smoke-$(date +%s)-${RANDOM}@example.com"

echo "health and docs"
req GET /livez; check "GET /livez is 200" is 200
req GET /readyz; check "GET /readyz is 200 with the database up" is 200
req GET /openapi.yaml; check "GET /openapi.yaml is served" is 200
req GET /docs/; check "GET /docs/ (Swagger UI) is served" is 200
check "every response has X-Request-ID" test -n "$(header X-Request-ID)"
check "security headers are set" test "$(header X-Content-Type-Options)" = "nosniff"
req GET /api/v1/nope; check "unknown route is a JSON 404 with request_id" bash -c '[ "$0" = 404 ] && [ -n "$1" ]' "$STATUS" "$(field .request_id)"
req POST /livez; check "wrong method is 405 with Allow" bash -c '[ "$0" = 405 ] && [ "$1" = GET ]' "$STATUS" "$(header Allow)"

echo "input errors"
req POST /auth/signinup/code "" 'not json'; check "malformed body is 400" is 400
req POST /auth/signinup/code "" '{}'; check "body without a contact is 400" is 400
req POST /auth/signinup/code "" '{"phoneNumber":"+447400123456"}'
check "foreign number is refused and pointed to email" bash -c '[ "$0" = GENERAL_ERROR ] && grep -q email <<<"$1"' "$(field .status)" "$(field .message)"
req POST /auth/signinup/code "" '{"phoneNumber":"12345"}'; check "invalid number is refused" has_field .status GENERAL_ERROR

if [ "${BOT_CHECK}" = "turnstile" ]; then
	echo "bot check"
	NO_TOKEN=1
	req POST /auth/signinup/code "" "{\"phoneNumber\":\"${PHONE_LOCAL}\"}"
	NO_TOKEN=""
	check "a code request without a Turnstile token is refused" bash -c '[ "$0" = GENERAL_ERROR ] && grep -qi check <<<"$1"' "$(field .status)" "$(field .message)"
	check "no code was sent for it" test -z "$(code_for "$PHONE")"
fi

echo "phone sign-in"
req POST /auth/signinup/code "" "{\"phoneNumber\":\"${PHONE_LOCAL}\"}"
check "code sent to a local-format number" has_field .status OK
DEVICE="$(field .deviceId)"
PRE="$(field .preAuthSessionId)"
CODE="$(code_for "$PHONE")"
check "the code reached the SMS provider as E.164" test -n "$CODE"
WRONG=$(printf '%06d' $(((10#$CODE + 1) % 1000000)))
req POST /auth/signinup/code/consume "" "{\"deviceId\":\"${DEVICE}\",\"preAuthSessionId\":\"${PRE}\",\"userInputCode\":\"${WRONG}\"}"
check "a wrong code is rejected" has_field .status INCORRECT_USER_INPUT_CODE_ERROR
req POST /auth/signinup/code/consume "" "{\"deviceId\":\"${DEVICE}\",\"preAuthSessionId\":\"${PRE}\",\"userInputCode\":\"${CODE}\"}"
check "the right code signs in" has_field .status OK
ACCESS="$(header st-access-token)"
REFRESH="$(header st-refresh-token)"
check "access and refresh tokens come back in headers" test -n "$ACCESS" -a -n "$REFRESH"

echo "account"
req GET /api/v1/me; check "GET /me without a token is 401" is 401
req GET /api/v1/me "$ACCESS"
check "GET /me is a customer signed in by phone" bash -c '[ "$0" = 200 ] && [ "$1" = phone_otp ] && [ "$2" = customer ]' "$STATUS" "$(field .sign_in_method)" "$(field '.roles[0]')"
USER_ID="$(field .user_id)"
check "user_id is our UUID" test "${#USER_ID}" = 36
req GET /api/v1/me/identities "$ACCESS"; check "one sign-in method listed" has_field '.identities | length' 1
req POST /api/v1/me/identities "$ACCESS" "{\"phone_number\":\"${PHONE_LOCAL}\"}"; check "adding a phone already on the account is 409" is 409
req POST /api/v1/me/identities "$ACCESS" '{"email":"not-an-email"}'; check "adding an invalid email is 422" is 422
req POST /api/v1/me/identities "$ACCESS" '{"email":"a@b.co","nope":1}'; check "unknown fields are 400" is 400
req POST /api/v1/me/identities "$ACCESS" "{\"email\":\"${EMAIL}\"}"
check "adding an email sends a code (201)" is 201
LDEV="$(field .device_id)"
LPRE="$(field .pre_auth_session_id)"
LCODE="$(code_for "$EMAIL")"
req POST /api/v1/me/identities/verify "$ACCESS" "{\"device_id\":\"${LDEV}\",\"pre_auth_session_id\":\"${LPRE}\",\"code\":\"${LCODE}\"}"
check "verifying the code adds the email" bash -c '[ "$0" = 201 ] && [ "$1" = 2 ]' "$STATUS" "$(field '.identities | length')"
EMAIL_ID="$(field '.identities[] | select(.kind=="email") | .id')"

echo "email sign-in reaches the same account"
req POST /auth/signinup/code "" "{\"email\":\"$(tr '[:lower:]' '[:upper:]' <<<"$EMAIL")\"}"
EDEV="$(field .deviceId)"
EPRE="$(field .preAuthSessionId)"
ECODE="$(code_for "$EMAIL")"
req POST /auth/signinup/code/consume "" "{\"deviceId\":\"${EDEV}\",\"preAuthSessionId\":\"${EPRE}\",\"userInputCode\":\"${ECODE}\"}"
EACCESS="$(header st-access-token)"
req GET /api/v1/me "$EACCESS"
check "email sign-in (typed in capitals) is the same user" bash -c '[ "$0" = "$1" ] && [ "$2" = email_otp ]' "$(field .user_id)" "$USER_ID" "$(field .sign_in_method)"

echo "sessions"
req POST /auth/session/refresh "$REFRESH"
NEW_ACCESS="$(header st-access-token)"
check "refresh returns a new access token" bash -c '[ "$0" = 200 ] && [ -n "$1" ]' "$STATUS" "$NEW_ACCESS"
req POST /auth/signout "$EACCESS"; check "sign out is 200" is 200
req DELETE "/api/v1/me/identities/${EMAIL_ID}" "$NEW_ACCESS"; check "removing the email is 204" is 204
req GET /api/v1/me "$EACCESS"; check "the removed email's session is refused" is 401
req GET /api/v1/me/identities "$NEW_ACCESS"
PHONE_ID="$(field '.identities[0].id')"
req DELETE "/api/v1/me/identities/${PHONE_ID}" "$NEW_ACCESS"; check "removing the last sign-in method is 409" is 409

echo "admin"
req GET /api/v1/admin/invites "$NEW_ACCESS"
if [ -z "${OIDC_ISSUER:-}" ]; then
	check "admin routes are absent while admin sign-in is off (404)" is 404
else
	check "customers can't reach admin routes" bash -c '[ "$0" = 403 ] || [ "$0" = 404 ]' "$STATUS"
fi

echo "rate limits"
LIMITED=""
for _ in 1 2 3 4 5 6; do
	req POST /auth/signinup/code "" "{\"phoneNumber\":\"${PHONE_LOCAL}\"}"
	[ "$(field .status)" = GENERAL_ERROR ] && LIMITED="$(field .message)" && break
done
check "too many codes to one number are refused" test -n "$LIMITED"

echo "metrics"
METRICS="$(curl -s "http://localhost:${METRICS_PORT}/metrics")"
check "metrics are served on the private port" grep -q 'http_requests_total{method="GET",route="GET /api/v1/me",status="200"}' <<<"$METRICS"
check "SMS deliveries are counted" grep -q 'code_deliveries_total{channel="sms",result="ok"}' <<<"$METRICS"
check "email deliveries are counted" grep -q 'code_deliveries_total{channel="email",result="ok"}' <<<"$METRICS"
check "code-send refusals are counted" grep -q 'rate_limited_total{limit="code_send"}' <<<"$METRICS"
check "the database pool is reported" grep -q 'db_pool_connections{state="idle"}' <<<"$METRICS"
check "metrics are not on the public port" bash -c '[ "$(curl -s -o /dev/null -w "%{http_code}" "$0/metrics")" = 404 ]' "$BASE"

echo "account deletion"
req DELETE /api/v1/me "$NEW_ACCESS"; check "deleting my account is 204" is 204
req GET /api/v1/me "$NEW_ACCESS"; check "a deleted account's token is refused" is 401

echo "logs"
check "request log lines never contain a sign-in code" bash -c '! grep "request finished" "$0" | grep -q "$1"' "$LOG" "$CODE"
check "every request line has a request_id" bash -c '! grep "request finished" "$0" | grep -qv request_id=' "$LOG"
check "signed-in requests carry user_id" grep -q "user_id=${USER_ID}" "$LOG"

echo
echo "${PASS} passed, ${FAIL} failed"
[ "${FAIL}" -eq 0 ]
