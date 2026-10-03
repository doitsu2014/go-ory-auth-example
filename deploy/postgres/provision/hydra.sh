#!/bin/sh
# Idempotently creates the Hydra login role and database (ADR-0007 database
# per component). Runs as a one-shot compose job on every `up`, so it also
# works on Postgres volumes created before Hydra was added. The password is
# passed as a psql variable (:'var' quotes it safely).
set -eu

export PGPASSWORD="$POSTGRES_PASSWORD"
psql -v ON_ERROR_STOP=1 -h "${PGHOST:-postgres}" -U "${POSTGRES_USER:-postgres}" --dbname postgres \
  -v hydra_pw="$HYDRA_DB_PASSWORD" <<'SQL'
SELECT 'CREATE ROLE hydra LOGIN PASSWORD ' || quote_literal(:'hydra_pw')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hydra')
\gexec
-- Keep the password in sync with .env on every run.
ALTER ROLE hydra WITH LOGIN PASSWORD :'hydra_pw';
SELECT 'CREATE DATABASE hydra OWNER hydra'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'hydra')
\gexec
REVOKE ALL ON DATABASE hydra FROM PUBLIC;
SQL
echo "hydra database ready"
