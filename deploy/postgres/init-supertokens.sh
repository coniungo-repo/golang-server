#!/bin/sh
set -eu

: "${SUPERTOKENS_DB_USER:?set SUPERTOKENS_DB_USER}"
: "${SUPERTOKENS_DB_PASSWORD:?set SUPERTOKENS_DB_PASSWORD}"
: "${SUPERTOKENS_DB_NAME:?set SUPERTOKENS_DB_NAME}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v st_user="$SUPERTOKENS_DB_USER" \
  -v st_password="$SUPERTOKENS_DB_PASSWORD" \
  -v st_db="$SUPERTOKENS_DB_NAME" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'st_user', :'st_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'st_user') \gexec
SELECT format('ALTER ROLE %I PASSWORD %L', :'st_user', :'st_password') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'st_db', :'st_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'st_db') \gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', :'st_db') \gexec
SQL
